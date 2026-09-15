# Source review — Labeltron desktop enrollment plan

Companion to [DESKTOP_APP_PLAN.md](DESKTOP_APP_PLAN.md), the current proposal, and its [ASCII system diagrams](SYSTEM_DESIGN.md). `PLAN.md` is a historical draft, not the current architecture. The obsolete specification has been deleted.

## 1. Repositories and pinned revisions

The requested capture/cloud-sync URLs are two branches of the same repository. The existing capture clone was fetched, the Windows branch was added as a detached worktree, and the new stitcher repository was cloned. Existing clid/APID clones were fetched and their current main revisions opened in separate worktrees to verify direct HTTP contracts without disturbing the older checkouts.

| Repository / branch | Full reviewed commit | Local path | Role |
|---|---|---|---|
| [dustid/labeltron-two · jhodges/cloudsync](https://github.com/dustid/labeltron-two/tree/jhodges/cloudsync) | `e40522bbc619237b95ae8721b44861e960917d81` | `reference/labeltron-two` | Capture, cloud login and upload implementation |
| [dustid/labeltron-two · jhodges/wininstaller](https://github.com/dustid/labeltron-two/tree/jhodges/wininstaller) | `50cf335bc25fa9bef28bb7f46265253935bffd65` | `reference/labeltron-two--wininstaller` | Recommended base/release branch |
| [dustid/labeltron-two-stitcher · main](https://github.com/dustid/labeltron-two-stitcher) | `d78c82d9cc6ab0ac54f9dfe9a997a8fe6547effa` | `reference/labeltron-two-stitcher` | Actual Rust algorithm, manifests, cloud execution |
| [dustid/clid · main](https://github.com/dustid/clid) | `aa44cbfa396f45c329d751b68f7adaca33c62f66` | `reference/clid--review` | Reference implementation of APID calls and response validation; **not a shipped component** |
| [dustid/apid · main](https://github.com/dustid/apid) | `c7d52b30b672bfd11fa2fa9b1a543d5097d5e495` | `reference/apid--review` | Server routes, request schemas, authorization and reconciliation |
| [dustid/labeltron · jhodges/aws-app](https://github.com/dustid/labeltron/tree/jhodges/aws-app/aws-app) | `1c70e28f17d54f012aad871a78695c028b5a6769` | `reference/labeltron--aws-app` | Existing support-service snapshot: upload API and CDK configuration; not independently verified as the current deployed revision |

All source references below refer to these commits. Feature branches and deployed APIs may subsequently change.

### Existing historical references retained

The earlier `reference/clid` (`bdf80af`), `reference/apid` (`afb4c6e`), `reference/labeltron` (`f86ea2c`), `reference/labs-toolkit` (`8b91599`), `reference/labs-toolkit--apid-sdk-py` (`9050ff1`) and `reference/redirect-service` (`10bd97b`) checkouts were not removed or rewritten. The old Python stitcher and older SDK/spec assumptions are not the basis for this plan.

`reference/` is excluded by the existing `.gitignore`. No implementation changes, commits, pushes or deployments were made to the reference repositories.

## 2. Capture and cloud-sync evidence

Paths in this section are relative to `reference/labeltron-two`.

| File / location | What it establishes |
|---|---|
| `README.md` | PyQt6 capture architecture, six modes, simulator, burst/stream layouts, packaging and Vimba constraints |
| `src/labeltron/capture/layout.py:47,75` | Frame index prefixes, run/burst naming, flat streams versus nested bursts |
| `src/labeltron/capture/writer.py:39–64` | Image bytes are encoded then written directly; Unicode path handling; no temp-file/rename protocol yet |
| `src/labeltron/capture/writer.py:141–149` | `close()` joins with a timeout then clears its thread reference without asserting thread termination; sealing must not assume this proves a completed flush |
| `src/labeltron/capture/writer.py:192–207` | Saved/failed counters and per-image write behavior |
| `src/labeltron/capture/runner.py:224–238` | Run completion event follows writer close |
| `src/labeltron/cloud/sync.py:23–25,75–122` | 20 concurrent PUTs by default, paginated remote listing, presign batches of at most 500 |
| `src/labeltron/cloud/sync.py:125–183` | One-way diff/upload flow; credentials kept out of the separate S3 HTTP client |
| `src/labeltron/cloud/sync.py:149` | Remote object is skipped when `local_size <= remote_size`; no checksum comparison |
| `src/labeltron/cloud/sync.py:158–174` | All upload URLs are obtained before transfers; file content is read into memory per active upload; failed files are collected rather than retried in the function |
| `src/labeltron/cloud/auth.py:44–49,83–161` | Loopback Google OAuth with PKCE; returned credential contains ID token/email only, no persisted refresh/expiry lifecycle |
| `src/labeltron/cloud/client.py` | Current desktop Cloud API wrapper lists folders/auth status, not processing jobs |
| `src/labeltron/ui/panels/cloud_panel.py` | Cloud tab is a folder/count table, not a result review/enrollment UI |
| `src/labeltron/settings/model.py:90–100` | Google client configuration is in settings |
| `packaging/labeltron.spec`, `packaging/installer.iss`, `packaging/build_windows.ps1` | Existing Windows app/CLI installer and native camera library collection |

### Branch comparison checked

```text
git diff --stat origin/jhodges/cloudsync origin/jhodges/wininstaller

pyproject.toml            | 2 +-
src/labeltron/__init__.py | 2 +-
2 files changed, 2 insertions(+), 2 deletions(-)
```

The capture/cloud/packaging subtree diff is empty. We do not need to combine two independent applications or independently merge cloudsync into this Windows snapshot.

### Existing upload service

Relative to `reference/labeltron--aws-app/aws-app`:

- `app-cdk/api-lambda/index.js:321–407`: paginated object listing and signed PUT generation. Object keys are `${folder}/${fileName}`, not automatically `scans/<stream>/...`. Listing returns size/time, not a content checksum.
- `app-cdk/api-lambda/index.js:407–412`: existing folder/presign route table; no pipeline job/approval API.
- `app-cdk/lib/app-cdk-stack.ts:50–51`: upload bucket uses `RemovalPolicy.DESTROY` and `autoDeleteObjects: true` in this source snapshot. This is a production-infrastructure review item, not proof of the currently deployed bucket's settings.

## 3. Stitcher evidence

Paths are relative to `reference/labeltron-two-stitcher`.

### Core algorithm and output contracts

| File / location | What it establishes |
|---|---|
| `Cargo.toml` | Rust 2024; OpenCV, rayon, rxing, Polars and libjpeg-turbo dependencies |
| `README.md`, `QUICKSTART.md` | Reel/anchor/layout model, direction handling, tuning, reports, source-reported performance and known limitations |
| `src/matching.rs:28–41` | Nonrecursive `read_dir`, supported image extensions and filename sorting |
| `src/stitch.rs` | SIFT + Lowe ratio + one-to-one filter + RANSAC partial-affine; scale fallback; hard-cut full-resolution compositing |
| `src/stitch.rs::register_at` | Scale/rotation rejection and low-inlier warnings; not a calibrated per-label confidence probability |
| `src/decode.rs` and QR blocks in `src/bin/stitchin-complete.rs` | Classical/threshold/erosion/WeChat QR cascade; accept-prefix and fallback controls |
| `src/capture.rs` | Timestamp-gap diagnostics |
| `src/bin/stitchin-complete.rs:1544–1819` | Physical labels regrouped around anchors; each item assigned at most once; serial from the selected QR via reference manifest |
| `src/labels.rs:1–72,249–327` | Distinct schema-v1 `labels.json` and schema-v2 `crops/reel.json`; keyed identifier definitions, null missing values and relative crop paths |
| `configs/dust_small.yaml`, `configs/dust_large.yaml` | Current profile/template/layout/crop settings; 640×480 crops; long-label small-shield exports commented out |
| `src/bin/stitchin-complete.rs:1907–1920` | Writes label CSV, summary JSON and keyed Reel JSON as separate artifacts |

**Important:** `CLAUDE.md` explicitly forbids reading/changing files inside `manifests/`. Those files were not inspected. Reading config/source code describing the manifest interface does not validate actual stock/reference-data contents.

### Resume and quality hazards

- `src/bin/stitchin-complete.rs:794`: resume removes `crops/` before regenerating outputs.
- `src/bin/stitchin-complete.rs:973–991`: match checkpoint hashes frame names, direction, matching scale, methods and template/mask bytes; raw image contents are not included there.
- `src/bin/stitchin-complete.rs:1155–1168`: extraction fingerprint incorporates matching/config/boundary values; this is not a complete immutable artifact/image/model provenance record.
- `src/bin/stitchin-complete.rs:1380–1384`: an existing decodable composite is reused by filename. Changing source bytes or stitch configuration is not sufficient evidence that this composite is valid.
- `src/labels.rs:77–112` plus `src/bin/stitchin-complete.rs:1638,1664–1678`: sanitized serial/fallback is used as output path base. Duplicate or colliding serials can point separate crop jobs at the same file. Collision handling must happen before rendering, not only after enrollment begins.
- `src/bin/stitchin-complete.rs:1698,1850–1887`: “complete” is calculated before parallel crop rendering; crop failures patch JSON records but do not reconstruct the earlier CSV status.
- `src/bin/stitchin-complete.rs:2039–2058`: missing-crop banner is emitted, but the function still returns `Ok(())`.
- `src/checkpoint.rs:180–184,217–244`: loading tolerates a truncated trailing line, while append opens the original file without trimming that partial record first. The existing truncation test validates reading, not repeated resume-after-append safety.

These observations justify immutable attempts, importer checks and content-bound checkpoints. Except for the wrapper test below, they are code-review findings, not reproduced on real capture data in this session.

### Container/cloud execution

- `Dockerfile`, `docker/base.Dockerfile`: Linux/OpenCV-contrib runtime and baked-in local QR models; no Windows installer target established by this review.
- `.cargo/config.toml`: Homebrew paths and local `target-cpu=native`. **The Dockerfile does not copy this file into its build stage.** Do not misattribute local build flags to the cloud image.
- `docker/entrypoint.sh:81–123`: S3 assets/frames staging, pipeline invocation and final result sync. It attempts to upload after ordinary pipeline failures; hard kills/OOM cannot be assumed to run this cleanup.
- `k8s/README.md`, `k8s/job.yaml`, `k8s/run-job.sh`: one Job per run; x86 nodes; requested resource sizes; no automatic retry (`backoffLimit: 0`); ephemeral volume or explicitly retained PVC; 24-hour Job TTL.
- `k8s/README.md`: fresh ephemeral Jobs do not have old checkpoints. Restoring results or keeping a volume is required for cross-Job resume; excluding composites limits reuse.
- `QUICKSTART.md`: reports can be 2–3 GB for large runs. Treat this as a source-reported example; use lazy review data rather than assuming giant embedded HTML is suitable for the desktop.

### Confirmed wrapper exit-code bug

Location: [`docker/entrypoint.sh:57–76`](https://github.com/dustid/labeltron-two-stitcher/blob/d78c82d9cc6ab0ac54f9dfe9a997a8fe6547effa/docker/entrypoint.sh#L57-L76).

The command runs inside a piped brace group, followed by `echo "# exit $? ..."`. `PIPESTATUS[0]` observes the group's last successful echo, not the failed algorithm command.

A temporary directory supplied a stub `stitchin-complete` on PATH that printed a diagnostic and exited 42. The real entrypoint was invoked with `complete --name failure-probe`; `STITCHIN_RUNS` pointed inside the temporary directory.

Observed:

```text
Stub algorithm exit code: 42
Container wrapper exit code: 0
simulated algorithm failure
# exit 42 at <timestamp>
```

No algorithm was run; no image, protected reference data, S3 or APID access was involved. The temporary files were cleaned up and the cloned source was unchanged.

## 4. Direct APID contract evidence

Server paths relative to `reference/apid--review`; client paths relative to `reference/clid--review`.

### Server is authoritative

| Server file | Contract / behavior |
|---|---|
| `apps/apid/src/routes/v1/composite-tags.ts:37–221` | Collection/Reel list/create/detail; multipart `POST /reels/:reelCollectionId/labels` under `/api/v1/composite-tags` |
| `apps/apid/src/lib/domain/api/composite-tag.ts:160–167` | Reel creation uses `name`, optional `collectionId` and `expectedIdentifiers`; name is not unique |
| `apps/apid/src/lib/domain/api/composite-tag.ts:315–381` | Explicit members, fingerprint IDs, multipart JSON options/identifiers, image file/base64, explicit positive position, response shape |
| `apps/apid/src/lib/ops/composite-tags/create-label.ts:42–114` | External scan work before final locks; compatible same-Reel/same-position DUST reconciliation; otherwise conflict |
| `apps/apid/src/lib/ops/composite-tags/create-label.ts:116–140` | Explicit position collision versus next-free allocation |
| `apps/apid/src/lib/ops/composite-tags/_identifiers.ts` | Resolves DUST and value members, associates canonical printed Vlinks, attaches/reconciles members; non-DUST values can repeat |
| `apps/apid/src/lib/ops/composite-tags/_helpers.ts:137–154` | Humans need Team/org admin for enrollment setup; Service Account membership in the selected Team is explicitly accepted |
| `apps/apid/src/lib/ops/composite-tags/_helpers.ts:36–58` | Expected composition is a configurable completeness hint, not an input constraint |
| `apps/apid/src/routes/v1/tags.ts:17–37` and `apps/apid/src/lib/domain/core/scan/index.ts:62` | `POST /api/v1/tags/extract`, image file/base64 input |
| `apps/apid/src/lib/ops/tags/utils/enroll-dust.ts` | Extraction/enrollment invokes external scan service; duplicate-DUST reconciliation and classified failures |
| `apps/apid/src/lib/utils/vlink-code.ts` | Canonical origin/path validation and fixed-width, case-sensitive Base62 code ↔ UUID |
| `apps/apid/src/routes/v1/vlinks.ts:127–145` | Optional Vlink-detail enrichment route; independently authorized |

No generic `Idempotency-Key` handling was found in the inspected middleware and composite-tag creation path. This does not assert that no unrelated APID API implements idempotency.

### clid as a behavioral reference, not a dependency

- `crates/clid-cli/src/commands/auth/utils.rs:57–90`: `GET <authd>/api/auth/token` with `x-api-key`, then use returned token.
- `crates/clid-cli/src/client.rs:121–122`: `dust-ctx-org-id` and `dust-ctx-team-id` context headers.
- `crates/clid-client/src/apis/composite_tags.rs:391–422`: multipart fields `position`, `data`, `identifiers`, `humanReadable`, `qrValue`, `options`.
- `crates/clid-client/src/apis/composite_tags.rs:556–659`: actual route paths for Collection/Reel operations, Label creation, extraction and member addition.
- `crates/clid-cli/src/commands/labels.rs:1234–1312`: legacy one-image shortcut versus extract-each-image then explicit-member creation. Names/descriptions require explicit members.
- `crates/clid-cli/src/commands/labels.rs:1315 onward`: verify returned live members and indexing before checkpointing.
- `crates/clid-cli/src/labels/manifest.rs:20–25,42–43`: current CLI accepts schema v1/v2; applies its own 25 MiB image cap; checkpoint digest is described as semantic. The historical byte-identical-manifest-only assumption should not be copied into the new design.

**Critical distinction:** clid reads S3/local/HTTPS images and sends their bytes. The reviewed APID Label-create schema does not accept an `s3://` manifest or `imageUrl` field. The new worker must fetch the crop and construct the supported HTTP request.

## 5. Scope and verification limits

Completed:

- Cloned the new stitcher, fetched the requested capture branches and added the Windows worktree.
- Fetched current clid/APID and reviewed detached worktrees.
- Compared capture branches and traced capture → upload → stitching → keyed crops → APID request/response/reconciliation.
- Read the algorithm/cloud quickstarts and relevant implementation paths, rather than assuming the older Python stitcher applies.
- Reproduced the wrapper false-success behavior with an isolated stub.
- Wrote a proposal separating existing interfaces, required hardening and new workflow APIs.

Not performed:

- No real image stitching, protected manifest inspection or scientific association/quality evaluation.
- No full Rust/Python/server test suite or Windows build. Native OpenCV was not available through the default local `pkg-config` lookup; no environment/dependency installation was attempted for this planning task.
- No S3 listing/download/upload, EKS Job submission, production API enrollment or deployment changes.
- No validation of live bucket policy, ECR image contents, permissions, current endpoints, scan-service availability or reported throughput.

Those are explicit phase-0/pilot tasks, not implied successes. The architecture and schedule remain recommendations pending that validation.
