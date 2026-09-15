# Label Enrollment App — First Draft Plan

> **Superseded — retained for historical context.** See [DESKTOP_APP_PLAN.md](DESKTOP_APP_PLAN.md), [SYSTEM_DESIGN.md](SYSTEM_DESIGN.md) and [SOURCES.md](SOURCES.md) for the current source-reviewed proposal: the Rust `labeltron-two-stitcher`, verified S3 processing, and direct APID calls without clid. The Python-stitcher, CLI-sidecar and checkpoint assumptions below must not guide new implementation. The later Tauri/web-UI choice does **not** restore this draft: the current Python helper is capture-only, stitching stays in the proposed cloud pipeline, and clid is not bundled.

**Original status:** draft for review. Written after reading `dustid/labs-toolkit` (main +
`jhodges/apid-sdk-py`), `dustid/clid`, and `dustid/labeltron`.

Clones for reference live in `reference/` (gitignored).

---

## 1. What already exists

The pipeline you described is mostly built. It is just not packaged, and the
pieces do not talk to each other.

| Stage | What exists today | Where |
|---|---|---|
| Capture | Labeltron control software (Python, OpenCV, pyzbar, USB) | `dustid/labeltron` — `go.py` |
| S3 | Bulk **downloader** S3↔local, and a GDrive→S3 uploader | `labeltron/scripts/post_processing/` |
| Stitch | `ScanStitcher` — SIFT/ORB matching, graph stitch ordering, alpha blend, crop | `labs-toolkit/image_processing/stitcher` |
| Pre-process | Loupe distortion correction, crop/resize | `labs-toolkit/image_processing/pre_processing` |
| Enroll | `clid labels validate / enroll / report` with checkpoint + resume | `dustid/clid` |
| Orchestration | `enroll-labels.sh` — scanner folder → clid manifest → enroll | `clid/scripts/enroll-labels.sh` |

`enroll-labels.sh` is the single most valuable artifact here. **The desktop app
is essentially a GUI reimplementation of that script, plus the S3 and stitch
stages that the script assumes have already happened.** Read it before writing
any code.

### Two facts that shape the whole design

**clid enrolls directly from S3.** From the clid README:

```bash
clid labels enroll s3://label-scans/reels/0075/reel.json \
  --collection "August Production" --reel-number "0075" --indexing identifiable
```

Rows may carry `imagePath` (relative, resolved beside the manifest in the same
bucket), `imageUrl: s3://…`, or a presigned HTTPS URL. So once stitched images
are in S3, **nothing needs to come back down to the Windows box.** The app
uploads, then hands clid an `s3://` manifest.

**clid already owns resume.** Every row outcome is appended to
`<manifest>.clid-labels.jsonl`, and the checkpoint is pinned to the manifest's
sha256. Rerunning the same command retries only incomplete rows. The app must
not reinvent this — it should surface it.

---

## 2. Target architecture

```
┌──────────────────────────────────────────────────────────────┐
│  Labeltron Windows station                                   │
│                                                              │
│  Labeltron capture ──▶ C:\labeltron\data\<reel>\             │
│                             │                                │
│  ┌──────────────────────────▼─────────────────────────────┐  │
│  │  Label Enrollment App  (Tauri v2, React/TS + Rust)     │  │
│  │                                                        │  │
│  │  1. Discover reel      scan folder, parse source json  │  │
│  │  2. Upload raw     ──────────────────────────▶ S3      │  │
│  │  3. Stitch             stitchd sidecar (Python)        │  │
│  │  4. Upload stitched ─────────────────────────▶ S3      │  │
│  │  5. Write reel.json    clid manifest, byte-stable      │  │
│  │  6. Enroll             clid sidecar ──────────▶ APID   │  │
│  │  7. Report             tail .clid-labels.jsonl         │  │
│  └────────────────────────────────────────────────────────┘  │
│         │ spawns                    │ spawns                 │
│    stitchd.exe (PyInstaller)   clid.exe (Rust)               │
└──────────────────────────────────────────────────────────────┘
```

The app is a **supervisor for two sidecar processes**. It owns state, UI,
retries, and the manifest; it owns no algorithm and no API client.

That framing is why the recommendation is **Tauri v2, not Electron**:

- Tauri v2 has first-class sidecar support (`bundle.externalBin`) — shipping
  `clid.exe` and `stitchd.exe` inside the installer is a config line. Electron
  needs custom `extraResources` packaging and path juggling.
- `dice4-app` in `labs-toolkit/dice4_sdk/dice4-app` is already Tauri v2 +
  React 18 + TypeScript + Tailwind + shadcn + TanStack Query/Table, with
  `TAURI_APP_PLAN.md` documenting the Rust-owns-auth pattern. Copy that skeleton.
- clid is a Rust workspace. If we later want to skip process spawning, the Rust
  backend can depend on `clid-client` (or lift `clid-cli`'s `labels` module)
  directly. Electron closes that door.
- Installer size and Windows memory footprint on a factory PC.

The one real argument for Electron is team familiarity. If nobody wants to
maintain Rust, say so now — the plan below survives the swap, only §6 changes.

---

## 3. The seven stages, concretely

### Stage 1 — Discover

Point the app at a data root. It finds each reel as a `<reel>.json` +
`<reel>/` pair (the layout `enroll-labels.sh` documents):

```
<target>/
  <reel>.json                                # source manifest, JSON array
  <reel>/
    <SERIAL>/<SERIAL>-large-shield.png
```

Each source row:

```json
{ "serial-number": "IBG9OR6",
  "decoded-qr": "https://v.dustid.local/40oFQ3SKZ5JZbrABteRrc5",
  "large-shield-cropped-filepath-1": "IBG9OR6/IBG9OR6-large-shield.png" }
```

The numbered `-1`, `-2`, `-N` suffix is where stitching enters: **N captures per
label.** `enroll-labels.sh` sidesteps stitching by taking the lowest-numbered
one. We want to combine them.

> ⚠️ **Open question 1 — the most important one in this document.** I could not
> find the code that writes this layout. `labeltron/go.py` writes
> `image_{counter}.jpeg` into the working directory, and `main.py` is a stub.
> Meanwhile `labeltron/scripts/f2f/reformat_jh_csv.py` consumes a *different*
> shape — a `labels.csv` with `counter, serialnumber, url, scan, qr_image`.
> **We need one screenshot of a real capture folder before committing to a
> parser.** Everything downstream keys off this.

### Stage 2 — Upload raw captures

Mirror `<reel>/` to `s3://<bucket>/<prefix>/<reel>/raw/`. Content-addressed
skip: `HEAD` the key, compare size + ETag, skip if identical. This is exactly
what `s3_bulk_downloader.py::_should_download_file` does in reverse — port that
logic.

This stage is **archival**, not an input to stitching. Stitching reads the local
originals. Uploading first means a crash after capture never loses data.

### Stage 3 — Stitch

Per label: N shield images → 1 blended PNG.

```python
from stitcher import stitcher
s = stitcher.ScanStitcher(scans=paths, config={
    "descriptor": "SIFT",
    "stitch_order_method": "graph",
    "crop_transparent_region": "inner_rectangle",   # no empty regions
    "transform_config": {...},
})
s.calc_fingerprints()
s.calc_stitch_order()
rgba, coverage = s.blended_image
```

> ⚠️ **Open question 2 — is stitching actually wanted per-label, or per-reel?**
> The stitcher's docstring says "join multiple scans of a tag to obtain one large
> image of the tag" — per-tag. But the README example globs a whole directory.
> Confirm the intended grouping.

**The stitcher is research code and is not safe to run headless as-is.** Four
concrete defects, all in `image_processing/stitcher/src/stitcher/stitcher.py`:

1. `import pdb; pdb.set_trace()` at lines **191, 257, 431** — inside bare
   `except` blocks. In a GUI sidecar with no TTY these **hang the process
   forever** on any transform failure. Must be replaced with raised exceptions.
2. `blended_image` is a **plain `@property`, not cached**. Every access
   recomputes the entire blend. Read it once into a local.
3. No file output, no CLI, no `main()`. In-memory `np.ndarray` only.
4. No quality gate. If SIFT finds too few inliers the blend silently degrades.
   `TransformPassCriteria` (`Nin_min`, `det_min`, `det_max`) exists — surface a
   per-label pass/fail and confidence score to the UI so an operator can reject
   before enrolling. A bad stitch enrolled is far more expensive than one caught.

**Deliverable: a new `stitchd` sidecar** wrapping the library:

```
stitchd --in <reel-dir> --out <stitched-dir> --manifest <source.json> \
        --config stitch.toml --jsonl-progress
```

- one JSON line per label on stdout: `{serial, status, outPath, nScans,
  inliers, confidence, elapsedMs}` — the app tails this for progress;
- an idempotent output dir so a rerun skips completed labels;
- packaged with PyInstaller into `stitchd.exe`, shipped as a Tauri sidecar.
  OpenCV + igraph + largestinteriorrectangle bundle to roughly 150–250 MB.
  Verify early — this is the biggest packaging risk in the project.

> **Cloud alternative.** If the Windows box turns out to be too slow, `stitchd`
> becomes an S3-triggered Lambda/Batch job and the app polls for output. Keep
> the CLI contract above as the seam so this is a swap, not a rewrite. Do not
> build it for v1 — you would pay a full round trip to S3 and back for no gain
> while the originals are sitting on local disk.

### Stage 4 — Upload stitched

Same uploader as Stage 2, to `…/<reel>/stitched/<SERIAL>.png`. PNG or JPEG,
**≤ 25 MiB each** — a clid hard limit. Check dimensions of a real stitch against
this early.

### Stage 5 — Generate the clid manifest

Transform the source rows into clid's schema:

```json
{ "schemaVersion": 1,
  "labels": [
    { "imagePath": "stitched/IBG9OR6.png",
      "humanReadable": "IBG9OR6",
      "qrValue": "https://v.dustid.local/40oFQ3SKZ5JZbrABteRrc5" }
  ] }
```

Written to `s3://…/<reel>/reel.json`, beside the `stitched/` prefix so relative
`imagePath` resolves in-bucket.

**This must be byte-stable.** clid pins its checkpoint to the manifest's
sha256; a manifest that re-renders differently invalidates resume and restarts
the reel. The Python generator inside `enroll-labels.sh` gets this right —
`json.dumps(..., indent=2, ensure_ascii=False) + "\n"`, source order preserved
— and also carries security checks worth keeping verbatim:

- reject absolute paths and Windows drive letters;
- resolve symlinks, then assert the target stays inside the reel directory;
- reject duplicate serials and duplicate image paths;
- fail on an empty or non-array source.

Port all of it. If Rust's `serde_json` pretty printer diverges from Python's on
any input, the app must be the one authority — pick one renderer and write a
golden test.

### Stage 6 — Enroll

```
clid --api-url <…> --auth-url <…> --org-id <…> --team-id <…> \
     labels enroll s3://…/<reel>/reel.json \
     --collection "<name>" --reel-number 0075 \
     --indexing identifiable --concurrency 4 --yes \
     --checkpoint <local>/<reel>.clid-labels.jsonl
```

- `--yes` is mandatory — there is no TTY to answer the prompt.
- `--max 1` first, always: a one-row smoke test surfaces auth, org, collection,
  and image-format problems before 5,000 rows do. Make this a UI step, not a
  power-user flag.
- `--dry-run` backs the app's "Review" screen.
- Run `clid labels validate` before enrolling. It contacts nothing and catches
  bad manifests and unreadable images for free.

### Stage 7 — Progress and reporting

clid has no `--output json` for label commands, so **do not parse stdout.** The
machine-readable surface is the checkpoint JSONL
(`clid/crates/clid-cli/src/labels/ledger.rs`), one record per line:

```jsonc
{ "recordType": "session", "schemaVersion": 2, "manifestSha256": "…",
  "orgId": "…", "teamId": "…", "collectionId": "…", "collectionName": "…",
  "reelCollectionId": "…", "reelNumber": "0075", "indexing": "identifiable",
  "labelCount": 500, "createdAtUnix": 1757... }

{ "recordType": "row", "position": 1, "rowSignature": "…",
  "imagePath": "stitched/IBG9OR6.png",
  "status": "created" | "already_enrolled" | "error",
  "compositeTagId": "…", "dustTagId": "…", "qrTagId": "…",
  "error": {...}, "recordedAtUnix": 1757... }
```

Tail it with a file watcher → `position / labelCount` progress, a live error
list, and per-label tag ids. On completion run `clid labels report` to reconcile
local checkpoint against remote state, and show that as the authoritative
result. Failure recovery is one button: **Resume** re-runs the identical
command.

---

## 4. UI

Six screens, one linear flow with a persistent reel sidebar.

1. **Connect** — environment (prod / local / non-prod), org + team, `clid auth
   status` indicator, `clid auth login --service-account` button, S3 bucket and
   prefix, "Test connection".
2. **Reels** — discovered reels with per-stage state chips
   (`captured · uploaded · stitched · enrolled`), label counts, last activity.
3. **Review** — thumbnail grid of a reel's labels: source captures, stitched
   result, stitch confidence, QR value, serial. Reject/re-stitch individual
   labels here. This is where a bad stitch gets caught.
4. **Run** — the pipeline as a stepper with a live log pane. Per-stage
   progress bars fed by the two JSONL streams. Pause / Cancel / Resume.
5. **Results** — `clid labels report` output, error table with the stored clid
   error payload per row, CSV export, "Retry failed rows".
6. **Settings** — stitcher config (descriptor, pass criteria, crop mode),
   concurrency, indexing mode, paths, log level.

Design notes: this is an industrial tool on a factory floor. Prioritise
legibility at distance and unambiguous state over polish. Every long-running
action needs a visible cancel. Never block the UI thread on a sidecar.

---

## 5. Auth and secrets

**DUST.** Use a clid **Service Account**. clid stores the API key in the Windows
Credential Manager (`clid-cli/src/commands/auth/store/windows.rs` exists — the
support is in the source). The app never reads, stores, or logs the key: it
shells out to `clid auth login --service-account`, and thereafter relies on
clid's own token exchange. For an unattended station, inject
`DICE4_API_KEY` into the child process environment only, from the OS credential
store — never a `.env` file, never a command argument.

**AWS.** `labeltron/.env.example` has bare `AWS_ACCESS_KEY_ID` /
`AWS_SECRET_ACCESS_KEY`. Long-lived static keys on a shared factory PC are the
weakest link in this design. Prefer, in order: SSO / IAM Identity Center, an
IAM role via `AssumeRole`, or at minimum a dedicated key scoped to
`PutObject`/`GetObject`/`HeadObject` on exactly one bucket prefix. Store it in
the Windows Credential Manager, not on disk. Flag this to whoever owns the
station.

---

## 6. Blockers and open questions

Ordered by how much they can hurt.

| # | Issue | Impact | Fix |
|---|---|---|---|
| 1 | **What does Labeltron actually write?** Unresolved — see Stage 1. | Blocks the parser, i.e. everything | One real capture folder listing |
| 2 | **clid ships no Windows binary.** `dist-workspace.toml` targets only macOS ×2 and Linux ×2 — no `x86_64-pc-windows-msvc`. Latest release v0.5.0. | Blocks shipping | Add the target to `dist-workspace.toml`; the Windows keyring code already exists. Small PR to `dustid/clid` — raise it now, not in week 6 |
| 3 | **`pdb.set_trace()` ×3 in the stitcher** | Hangs the sidecar with no error | Replace with typed exceptions; PR to labs-toolkit |
| 4 | **PyInstaller bundle of OpenCV + igraph** | Installer size, possible DLL hell | Spike in week 1 — build a hello-world `stitchd.exe` on Windows before anything else |
| 5 | Stitch grouping: per-label or per-reel? | Changes the data model | Ask |
| 6 | Stitched image size vs clid's 25 MiB cap | Late failure at enrollment | Measure one real stitch |
| 7 | Manifest byte-stability across Python and Rust | Silently breaks resume | Golden test with non-ASCII and unicode serials |
| 8 | Does stitching need `pre_processing` distortion correction first? There is Loupe v1.1 calibration data in the repo. | Stitch quality | Ask whoever owns the algorithm |

---

## 7. Suggested sequencing

Each phase ends with something demonstrable.

**Phase 0 — De-risk (≈1 week).** Nothing but the three spikes: build clid for
Windows; PyInstaller `stitchd.exe` on Windows; run the real end-to-end flow by
hand on one reel using `enroll-labels.sh`. **Do not start the app until the
manual pipeline works once.** If it cannot be done by hand it cannot be
automated.

**Phase 1 — `stitchd`.** Wrap `ScanStitcher` in the CLI from Stage 3. Fix the
`pdb` calls. JSONL progress, idempotent output, quality scores. Test against
`image_processing/stitcher/data/sample_input`. Ship it standalone — it is
useful on its own, independent of the GUI.

**Phase 2 — App skeleton.** Fork `dice4-app`'s Tauri v2 setup. Screens 1 and 2.
Reel discovery and parsing. No pipeline yet.

**Phase 3 — S3.** Upload with skip-if-identical, resume, progress. Screens 2
and 4 wired to real work.

**Phase 4 — Stitch integration.** `stitchd` as a sidecar. Screen 3, the review
grid. First point where the app produces something an operator judges.

**Phase 5 — Enroll.** Manifest generation with the ported safety checks. clid
sidecar. Checkpoint tailing. Screens 4, 5. Smoke-test-first flow.

**Phase 6 — Ship.** Windows installer, code signing, auto-update
(`tauri-plugin-updater`), operator runbook, one real production reel end to end.

---

## 8. What I would push back on

**"Upload to S3, then stitch."** As stated this implies stitching consumes the
S3 copy. It should not, while the originals are on local disk — that is a full
round trip for nothing. Upload raw for durability, stitch locally from
originals, upload the stitched result, enroll from `s3://`. Same ordering you
described, but S3 is the archive and handoff, not the stitcher's input. Move
stitching to the cloud only when the station's CPU is measurably the bottleneck
— and keep the `stitchd` CLI contract so it is a swap.

**Don't rebuild what clid does.** Retry, resume, checkpointing, concurrency,
image validation, S3 fetching, and Collection/Reel creation are all in clid and
are more carefully written than a first pass in the app would be — read
`labels/manifest.rs` (43 KB of validation) before deciding to reimplement any
of it. The app's job is discovery, stitching, upload, and a human-readable face
on `clid labels enroll`.
