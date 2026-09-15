# Labeltron desktop enrollment — recommended design and implementation plan

**Status:** Tauri + web UI with a retained Python capture engine selected by the user; the remaining architecture is a proposal based on source-code review, not a production-validated design. See the [S00 decisions](plans/S00-pilot-scope.md).
**Source baseline:** capture branches through September 2, 2026; stitcher and clid through September 14; APID through September 11. Exact commits and evidence: [SOURCES.md](SOURCES.md).

**Architecture diagrams:** [SYSTEM_DESIGN.md](SYSTEM_DESIGN.md).
**Implementation slices and TODOs:** [IMPLEMENTATION_SLICES.md](IMPLEMENTATION_SLICES.md).

This plan replaces the earlier design; `PLAN.md` is retained only as historical context and the obsolete specification has been deleted. Earlier drafts used the wrong stitcher for this workflow and proposed shipping `clid`. **This design uses `dustid/labeltron-two-stitcher` and direct APID HTTP calls. No clid executable is required.**

## 1. Recommendation

**Build a Tauri v2 desktop shell with a bundled web UI, retain Labeltron's Python camera/capture engine as a supervised local helper, run the existing Rust stitcher in the cloud beside S3, and add a durable workflow service that enrolls through APID's existing endpoints.**

The user has selected Tauri + web UI and reuse of the Python capture engine. React + TypeScript + Vite is the recommended frontend stack, not yet a separately approved framework decision. A browser-hosted product and cross-platform camera support are not implied by this choice.

The operator still gets one end-to-end desktop experience:

> New run → capture → verified S3 upload → stitch → review → approve → enroll → reconcile/report.

The web UI owns presentation. Tauri's native Rust host owns scoped local commands, helper supervision, operator credentials, cloud HTTP and the upload journal. The Python helper owns camera access, capture, QR logging and sealed raw files. The backend owns long-running cloud jobs and durable enrollment. The cloud stitcher owns image reconstruction. APID owns Label/Reel identity and DUST enrollment.

This split means:

- Capturing never competes with stitching for workstation CPU, memory or disk.
- Closing the desktop does not interrupt a cloud stitch or an approved enrollment.
- A second authorized workstation can review an existing S3 run without downloading raw frames.
- We reuse Python camera recipes, acquisition, simulator and writer logic, plus the Linux stitcher container. The web UI, helper protocol and Tauri installer are new work; the old PyQt/Inno installer is only a packaging/licensing reference.
- The enrollment worker calls APID directly; it does **not** invoke clid, scand or an internal database.
- No AWS access keys, Kubernetes credentials or enrollment service-account key need to be installed on the station.

**Direct API calls need not originate on the desktop.** The recommended implementation makes them from a cloud worker on the desktop's behalf. If the requirement is specifically “all enrollment HTTP requests originate on Windows,” the same adapter can run there; see §15. That is a smaller infrastructure option, but jobs then depend on that workstation staying online.

## 2. What the repositories actually implement

### 2.1 Capture and sync are already one application

The two Labeltron links are branches of the **same repository**, not two separate apps. At the fetched commits, `jhodges/wininstaller` includes the cloud-sync work; its diff against `jhodges/cloudsync` is only the version bump in two files.

Existing components:

- Python/PyQt6 UI, Alvium/Vimba camera integration, simulator, six acquisition modes.
- Background frame writer and per-frame QR decoding.
- Browse and Cloud tabs, YAML settings, preflight diagnostics.
- Google desktop OAuth/PKCE login and presigned S3 uploads.
- PyInstaller/Inno Setup Windows packaging.

These describe the **existing** app, not the selected desktop shell. `capture/runner.py::BurstRunner`, callback events in `capture/events.py`, `camera/protocol.py`, `camera/simulator.py`, `runtime.py` and `preflight.py` provide a headless reuse seam. The current `cli.py` already calls that core without Qt, but prints human-readable output; it is not the new IPC protocol. Preserve core behavior and golden camera tests rather than translating the hardware engine to Rust or JavaScript.

The capture layouts are:

```text
<captures>/stream_<timestamp>/
  frame_000000_<timestamp>.png
  frame_000001_<timestamp>.png
  QR.txt                         # when QR logging is enabled

<captures>/burst_<timestamp>/
  <burst-timestamp>/
    frame_000000_<timestamp>.png
    ...
    QR.txt
```

**Do not infer one physical label from one folder.** Continuous streams are first-class, and the stitcher reconstructs label boundaries from the imagery.

### 2.2 The requested stitcher is Rust, not the older Python library

`labeltron-two-stitcher` builds `stitchin-complete` and supporting diagnostic tools. Its pipeline:

1. Loads a chronologically sorted, **flat** directory of frames.
2. Detects feed direction and matches anchor/content templates.
3. Finds boundaries and extracts features from frames.
4. Uses SIFT, one-to-one matches and RANSAC partial-affine registration to reconstruct seam-spanning features, retrying at finer scales when necessary.
5. Uses **hard-cut compositing**, not alpha blending. Preserve the algorithm's image semantics.
6. Decodes QR codes through classical, threshold/erosion and local WeChat model fallbacks.
7. Regroups features around anchors into physical labels using the configured layout.
8. Resolves serials using the configured QR→serial CSV, checks the scanned manifest range, renders standardized crops and reports warnings.

Current supplied profiles render **640×480 shield crops**. Those crops—not a panorama of the whole reel—are the enrollment images.

Outputs include:

```text
runs/<name>/
  labels.csv                     # label order, identity, feature/source diagnostics
  labels.json                    # stitcher summary, schemaVersion 1
  crops/reel.json                # keyed identifier manifest, schemaVersion 2
  crops/<serial-or-fallback>/...png
  stitched/lbl_<frame-a>_<frame-b>.png
  frames.csv
  config.yaml
  run.yaml
  performance.json
  warnings.txt
  report.html
  checkpoints/matches.json
  checkpoints/items.jsonl
```

`labels.json` and `crops/reel.json` are **different contracts**. Prefer `crops/reel.json` for enrollment mapping and the other artifacts for review/provenance.

The long-label profile extracts three shield occurrences, but its current `reel.identifiers` exports **only the large shield** plus TEXT and QR; both small-shield identifiers are commented out. Support an arbitrary approved DUST-slot list. Do not accidentally enroll every generated crop or silently select the first image.

### 2.3 Cloud execution already has a useful foundation

The stitcher has Docker/Depot builds, ECR image publishing configuration, EKS Job templates, S3 staging through `s5cmd`, logs and retained/ephemeral volume options.

Its intended cloud flow is already:

> S3 frames/assets → Linux job → stitcher → S3 results.

However, this is currently an operator/script interface, **not a desktop-facing job API with durable approvals, authorization and enrollment history**. That is the main new backend work.

### 2.4 APID already supports the required enrollment operations

Current clid source demonstrates two supported HTTP paths:

- One unnamed DUST crop + optional serial/QR: one multipart Label-create request.
- Named identifiers or multiple DUST crops: extract each crop, then create one Label from explicit identifier members.

APID provides Collection/Reel creation, explicit positions, Label enrollment, duplicate-DUST reconciliation and Reel reporting. We can reuse that contract without the CLI. Details are in §8.

## 3. Architecture and technology choices

```text
WINDOWS STATION
  Tauri v2 + bundled web UI (React/TypeScript/Vite proposed)
             | typed commands / bounded events
             v
  Native Rust host --------------------> Python capture helper
  - helper supervision                   - existing camera/simulator
  - OS credentials + cloud HTTP          - acquisition + QR + writer
  - SQLite upload journal                - flush + sealed raw files
             |                                     |
             +<------- immutable inventory --------+
             |
             | HTTPS: identity, runs, progress, approval
             v
CLOUD
  Workflow API + scheduler + enrollment worker (TypeScript)
  PostgreSQL jobs, leases, audit, outbox
             |                            |
             | launch/reconcile           | direct authenticated HTTP
             v                            v
  Rust/OpenCV stitcher job             AuthD + APID
  existing Linux image                 Collections / Reels / Labels
             |                            ^ crop bytes
             v                            |
  S3 raw / assets / results ---------------+
             ^
             +---- signed streaming uploads from native host
```

| Concern | Choice | Reason |
|---|---|---|
| Desktop shell | Tauri v2 + bundled web UI; React/TypeScript/Vite proposed | Selected product direction; native host isolates filesystem, process and credential access from the renderer |
| Local capture | Headless Python helper reusing `labeltron-two` core | Preserve camera behavior and simulator/golden tests; a versioned IPC adapter is new work |
| Cloud algorithms | Keep Rust/OpenCV container | Correct repository and existing deployment path; independent release/versioning |
| Workflow backend | TypeScript HTTP service and workers | Fits the surrounding APID/upload-service ecosystem; typed OpenAPI client where practical |
| Cloud state | PostgreSQL, with transactional outbox and leased jobs | Durable run/approval/row state; no need for separate Redis and SQS in the first version |
| Job execution | Existing EKS Job model **if that platform is operated already** | Reuse the supplied infrastructure; see alternative below |
| Local persistence | Rust-owned SQLite plus Python-written sealed run manifests | One upload-journal writer; renderer/helper do not share writable DB access; cloud state remains authoritative |
| APID integration | Small typed direct-HTTP adapter | No shell commands or CLI-output parsing |
| Artifact storage | Private S3, versioned references and content digests | Durable, auditable input/output and efficient remote review |
| Progress | Polling initially; resumable SSE later | Simple recovery; no requirement for a persistent desktop connection |

Do not create a microservice per pipeline stage. API, scheduler, importer and enrollment worker can be roles in one backend deployment/codebase. Keep the algorithm container separate because of its native dependencies and release cadence.

**If EKS is not actually available, use the same container on AWS Batch/ECS rather than introducing a Kubernetes platform just for this app.** Infrastructure availability is a phase-0 decision; the desktop/API contract does not change.

### Selected Tauri boundary

The earlier Qt-extension recommendation is superseded by the user's Tauri/web-UI decision. Replace the presentation layer, **not** the camera driver, trigger recipes or cloud stitcher. The Python helper is for capture, not a local stitcher or clid wrapper.

This adds real scope: a packaged headless helper, versioned IPC, bounded preview delivery, native process lifecycle, web UI capture controls and WebView2-aware installation. Prove a minimal Tauri shell with a fake helper and then a packaged simulator helper early after S01, rather than discovering Windows packaging problems at release. See the S10/S17 child gates in [IMPLEMENTATION_SLICES.md](IMPLEMENTATION_SLICES.md).

## 4. Operator journey

1. **Sign in / preflight.** Show camera, disk, cloud connectivity and destination-access status separately. Permit capture when cloud services are unavailable.
2. **New run.** Choose project, label profile, printed reel number and, if known, expected count/range. Record station/operator and camera settings. Assign a UUID independent of the folder name.
3. **Capture.** Web controls invoke the retained Python engine through Tauri; preserve validated camera settings and acquisition semantics. Show received, saved, dropped and failed counts. Stop acquisition, flush writers, then explicitly seal the capture.
4. **Upload.** Resume verified object transfers to S3. “Uploaded” means every sealed input is verified, not “folder exists.”
5. **Process.** Select a versioned approved profile and start stitching; optional auto-start after verified upload. Show queue and per-phase progress.
6. **Review.** Display reconstructed labels, required crop slots, serial/QR, direction, source frames and warnings. Filter to exceptions; inspect first/last labels and samples across the reel.
7. **Approve.** Freeze the chosen output revision, identities, positions, destination and indexing policy. Unresolved required labels block enrollment by default.
8. **Pilot enrollment.** Clearly labeled as a **real write**, enroll one approved label at its final position. Inspect returned members and perform a DUST verification check with the approved test procedure where available.
9. **Enroll remainder.** Bounded concurrency, progress by verified Label outcomes, pause/resume and explicit conflicts.
10. **Reconcile/report.** Compare every intended position/member against APID, then mark complete. Export a report and links to the cloud artifacts/APID Reel.

A run may be imported from an existing S3 prefix. It goes through the same inventory/sealing and validation steps; it cannot jump directly to “ready to enroll.”

## 5. Input contracts, sealing and S3 layout

### 5.1 Separate capture, processing attempt and APID Reel

They are not interchangeable:

- **Capture run:** an immutable ordered set of raw frames from one acquisition.
- **Processing attempt:** one algorithm/profile/input version combination; several attempts may reference the same run.
- **Reviewed enrollment plan:** approved labels, crops and explicit positions for a destination Reel.
- **APID Reel:** a persistent server entity identified by UUID, potentially supplied by an operator.

A printed reel number is a display value, not an identity. A processing retry must not create a new APID Reel.

For the first pilot, use one capture run per destination Reel. Design identifiers now so interrupted/rescanned segments can be introduced later without overloading a folder name.

### 5.2 Seal locally before declaring upload complete

New `capture-manifest.json` contains:

```json
{
  "schemaVersion": 1,
  "runId": "<uuid>",
  "stationId": "<station-id>",
  "mode": "free_stream",
  "startedAt": "<UTC timestamp>",
  "finishedAt": "<UTC timestamp>",
  "savedFrames": 11000,
  "droppedFrames": 0,
  "failedWrites": 0,
  "frames": [
    {
      "sequence": 0,
      "path": "frame_000000_<timestamp>.png",
      "bytes": 4200000,
      "sha256": "<digest>",
      "capturedAt": "<UTC timestamp>"
    }
  ]
}
```

Illustrative only: a real manifest lists every frame. Store camera/profile settings and capture timestamp semantics alongside it. Use relative safe paths; retain original names even when staging introduces new names.

Implementation rules:

- Write image files using temporary names and atomic rename; discovery ignores temporary files.
- Seal only after acquisition, frame writing and relevant QR logging actually finish. The current writer's timed join is not sufficient proof of completion; surface a timeout/error rather than declaring the run sealed while a thread may still be writing.
- Hash encoded bytes at write time or in a low-priority post-capture pass, never by blocking the acquisition callback.
- Persist writer loss counters. Timestamp-gap detection is supporting evidence, not a substitute for known dropped frames.
- Crashed/open runs require an explicit recovery inventory and operator disposition before sealing.
- A metadata edit does not mutate raw data. Additional capture creates a new run/segment.

### 5.3 Verified upload protocol

1. Register run/project and immutable input-manifest digest with the backend.
2. Request presigned PUTs in bounded windows, respecting the existing 500-file batch ceiling.
3. Stream file bytes with bounded concurrency; checkpoint each successful object.
4. Use validated S3 checksums or a backend-verified digest protocol, plus exact byte lengths. **Do not treat ETag as SHA/MD5 universally.** Multipart uploads and encryption change ETag semantics.
5. Refresh expiring credentials/URLs and retry transient failures with backoff/jitter.
6. Submit a completion request. Backend verifies inventory, expected checksums/version IDs and ownership; only then transitions the run to `raw_verified`.
7. Publish the completion marker last and enqueue processing through the transactional outbox.

For ordinary frame sizes, per-file PUT retries are sufficient; “resumable upload” initially means resume between files, not partial-byte resume within a 4 MB frame.

S3 does not provide a directory transaction. A sealed manifest plus verification is the commit boundary. S3 events may wake the scheduler, but arbitrary frame-created events must never start stitching.

### 5.4 Storage namespace

Proposed canonical layout:

```text
s3://<private-bucket>/projects/<project-id>/
  captures/<run-id>/
    raw/<original-relative-paths>
    capture-manifest.json
    raw-complete.json
  assets/<asset-bundle-sha>/
    configs/... templates/... masks/... approved-reference-data/...
  processing/<run-id>/<attempt-id>/
    algorithm/                   # original stitcher outputs
    result-manifest.json         # validated artifact inventory and quality findings
    thumbnails/...
    logs/...
  enrollment/<plan-id>/
    approved-plan.json
    report.json
    report.csv
```

This namespace is **proposed**, not what cloudsync currently writes. The existing upload API uses `<folder>/<fileName>`; the stitcher defaults to `scans/<stream>/`. The run registry must store the actual bucket/prefix. Add an adapter for existing root-level uploaded folders and provision a compatible new-prefix upload route. Do not assume the current folder sanitizer accepts an arbitrary nested namespace.

Pin the algorithm image by digest and the assets by immutable bundle ID. Do not process production reels against mutable `edge` images or a changing shared assets prefix.

Seal exact object versions or enforce immutable/write-once keys. Outstanding presigned PUTs must not be able to change the selected input after approval. Legacy mutable prefixes need a version-pinned or immutable import snapshot; a later broad `s5cmd sync` of their latest contents is not a substitute.

### 5.5 Streams versus bursts

- Flat streams are the first supported production input and already fit `list_images`.
- A burst parent directory does **not** fit the current nonrecursive stitcher input.
- Add a staging adapter for approved burst workflows: order by capture metadata, retain burst boundaries, map originals to a flat collision-free sequence, and preserve timing/provenance.
- Do not stitch across acquisition discontinuities without evidence of overlap. Independent bursts may need independent processing plus an explicit Reel-position mapping.
- Existing data without a manifest gets a reviewed import manifest; filename order alone is not evidence that an interrupted or concatenated run is continuous.

## 6. Processing orchestration and algorithm integration

### 6.1 Job lifecycle

The scheduler creates a unique cloud Job for each leased attempt:

1. Fetch only the referenced sealed frames at their pinned versions/digests and the approved asset bundle. Extend the current staging wrapper to honor this inventory rather than blindly sync a mutable prefix.
2. Verify the staged inventory/digests.
3. Run `stitchin-complete` with an allowlisted profile, run name, direction and bounded `--jobs`.
4. Save outputs and diagnostics under the unique attempt prefix.
5. Import and validate artifacts into the workflow database.
6. Publish an output-complete marker only after required outputs are durably uploaded and validated.
7. Transition to `awaiting_review`, `needs_attention` or `failed`—not directly to enrollment.

Create Kubernetes objects through a backend SDK and closed parameter schema, not a desktop `kubectl` or user-controlled shell string. Watch actual Job/Pod conditions, exit status, heartbeat and artifact completion. Recover scheduler restarts by discovering Jobs using durable attempt IDs.

### 6.2 Two independent outcomes

Keep `executionStatus` separate from `qualityStatus`:

- Execution may succeed while some labels have no crop or QR.
- An optional diagnostic HTML failure must not erase valid crop results.
- A partial S3 upload or algorithm failure must never be presented as complete.

The current CLI deliberately returns success after reporting missing crops. Its CSV “complete” field is calculated before parallel crop rendering, so the importer must also check actual manifest entries, file existence, image decoding and failed-crop metrics.

### 6.3 Machine-readable result and progress

Retain existing artifacts unchanged for diagnosis. Add a versioned `result-manifest.json` with:

- Input digest, image digest, config/template/model/reference-data digests.
- Direction and processing timestamps.
- Label candidates with anchor/source references, decoded QR/serial provenance, selected crop slots and hashes.
- Structured findings: missing crop/QR, duplicate identity/path, capture gap, registration warning, out-of-reference-range, unresolved endpoint labels.
- Counts, phase timing and required artifact inventory.

Add JSONL algorithm events or an explicit progress callback/file as a small upstream enhancement. Until then, use phase-level state and `performance.json`; do not build business logic around ANSI-colored console messages or invent percent-complete values from line counts.

Example **new** event contract:

```json
{"schemaVersion":1,"attemptId":"<id>","sequence":42,"phase":"extract","completed":120,"total":1500}
```

Reconnection uses persisted event sequence IDs. UI labels should distinguish “queued,” “downloading,” “matching,” “extracting,” “rendering,” “uploading results” and “validating.”

### 6.4 Resume rules

Algorithm resume and application resume are separate:

- Application resume recovers a durable job/enrollment plan.
- Algorithm `--resume` can reuse checkpoints only with compatible staged inputs/assets and required composites.
- Fresh ephemeral volumes do not magically contain previous checkpoints. Either retain a single-writer volume or explicitly restore a validated checkpoint bundle from S3.
- Excluding `stitched/*` saves storage, but can prevent checkpoint reuse across fresh Jobs.
- `--resume` currently deletes/regenerates `crops/`. Never resume into a published/approved attempt directory. Use a separate scratch workspace and publish a new immutable output attempt.
- Initially allow only exact-version interrupted-run resume. Disable cross-input/config reuse until the invalidation defects in §13 are fixed.

Cancellation stops new work, signals the child and tries to preserve checkpoints/logs. SIGKILL/OOM/node loss cannot be assumed to run final-upload code: keep remote job status/central logs and periodically durable checkpoints if mid-run recovery is required.

## 7. Identity, review and quality gates

### 7.1 Preserve the correct label-to-crop association

A visually plausible stitch can associate a QR with its neighbor's shield if the profile layout is wrong. Therefore “every row has a crop” is insufficient.

Profile qualification requires a hand-labeled representative set covering:

- Both feed directions.
- First/last labels and partial leader/trailer frames.
- Seam-straddling QRs and shields.
- Faint printing, duplicate anchors, missing frames and capture interruptions.
- Correct serial↔QR↔shield association, not just decode/feature counts.

Qualify each label design and camera geometry separately. Template matching is not scale-invariant; a camera/ROI/resolution change requires review of profile compatibility.

### 7.2 Serial/QR policy

The new stitcher obtains serials from a supplied QR→serial manifest; it does **not** OCR the printed serial or automatically query Vlink aliases.

- Prefer the approved reference manifest for the matching label stock; retain its immutable version and checksum.
- If unavailable, an optional enrichment adapter can validate the canonical Vlink, decode its Base62 UUID and query APID `GET /api/v1/vlinks/{id}` for an alias. Access and alias-to-printed-serial equivalence must be confirmed; do not assume the older spec's casing convention is universal.
- Missing/conflicting identity is a review problem, not permission to borrow a neighboring QR.
- Validate the whole QR URL against the configured origin and canonical code format. The algorithm's `accept_prefix` is a useful first filter, not full identity validation.
- Never navigate to arbitrary decoded QR URLs for enrichment.
- TEXT is case-insensitive in APID; QR is exact/case-sensitive. Preserve the original QR and the provenance of any TEXT normalization.

The repository explicitly protects `manifests/`; its contents were **not read or changed** during this review. Production reference-data ownership and approved access must be arranged with its owner.

### 7.3 Positions

Persist these distinct concepts:

- Frame acquisition sequence.
- Label candidate index/anchor location in scan order.
- Printed-stock manifest ordinal, when available.
- Explicit destination APID Reel position.

Direction correction fixes layout/crop orientation; it does **not** establish the business position numbering on the Reel. Obtain that mapping before approval. Never sort by serial text or silently reverse/re-number positions after an enrollment starts.

APID accepts explicit positive positions and can represent gaps. Default v1 policy: unresolved required labels block whole-plan approval. If partial enrollment is later allowed, preserve positions and report gaps—do not compact remaining rows. Rescans are linked replacement candidates, not automatic appends.

### 7.4 Approval gates

Block enrollment when:

- Raw inventory is unsealed/unverified, or selected output is unpublished/incomplete.
- Required DUST slot is missing, null, corrupt, out-of-bounds, or has a duplicate path indicating possible overwrite.
- Required serial/QR is missing, malformed, conflicting or duplicated under the project's identity policy.
- Physical-to-APID position mapping is unresolved.
- An applicable capture-gap/association warning is unresolved.
- Target environment/Team/Reel or indexing mode is not explicitly selected.

APID allows some repeated non-DUST values and treats expected composition as a hint. The enrollment app's stricter label-stock policy must be enforced independently.

Do not fabricate a generic “confidence 0.91.” Show actual findings, template scores with their method/direction, registration diagnostics and an explainable pass/review/fail policy. APID extraction quality is a different signal from stitch quality.

No silent single-frame fallback, lossy conversion, downscaling or threshold relaxation. A scientifically approved alternate crop/profile may be selected explicitly with a new artifact digest and approval.

Freeze an approval digest over output revision, selected image hashes, identities, positions, identifier definitions, environment/org/Team/Reel and indexing. Any change invalidates approval. Keep reviewer identity, timestamp and reason.

## 8. Direct APID integration — exact supported workflow

These are **existing APID routes**, verified against the current server and clid client code. The proposed workflow routes in §10 are separate.

### 8.1 Authentication and context

The service-account flow currently used by clid is:

```http
GET <authd-base>/api/auth/token
x-api-key: <service-account-key>
```

The response supplies a `token`. APID calls then include:

```http
Authorization: Bearer <token>
Dust-Ctx-Org-Id: <organization-uuid>
Dust-Ctx-Team-Id: <team-uuid>
```

Keep a shared token manager that renews ahead of expiry and serializes renewal across worker concurrency. On 401, refresh once and retry/reconcile the same operation. Persistent 401 or a permission-denied 403 pauses the batch; it is not an unlimited-retry condition.

Route metadata calls setup/enrollment `team-admin`. The current implementation also authorizes a **Service Account that is a member of the selected Team**; do not unnecessarily give a machine global/admin access. Validate deployed entitlement, membership and feature availability in phase 0. Vlink lookup has its own authorization requirements.

### 8.2 Select/create the Collection and Reel

| Method | Path | Use |
|---|---|---|
| GET | `/api/v1/composite-tags/collections` | Paginated Collection picker |
| POST | `/api/v1/composite-tags/collections` | Create Collection, if explicitly requested |
| GET | `/api/v1/composite-tags/reels` | List/filter Reels, including by Collection |
| POST | `/api/v1/composite-tags/reels` | Create a Reel |
| GET | `/api/v1/composite-tags/reels/{reelCollectionId}` | Ordered labels, counts and warnings |

Create Collection JSON:

```json
{"name":"September Production","description":"Label enrollment"}
```

Create Reel JSON:

```json
{
  "name": "0075",
  "collectionId": "<collection-uuid>",
  "expectedIdentifiers": [
    {"tagType":"DUST","count":1},
    {"tagType":"TEXT","count":1},
    {"tagType":"QR","count":1}
  ]
}
```

Use `name`, **not** a fabricated `reelNumber` field in the APID request. Set DUST count from the selected identifier profile. Persist the returned `reel.collectionId`, which is the Reel UUID despite the field's name.

**Reel names are not unique.** Never resolve resume by name alone. Initial v1 may require selecting a pre-created Reel to reduce ambiguous-create failure handling; see §9.

### 8.3 One-image shortcut

For a single DUST crop without per-member metadata:

```http
POST /api/v1/composite-tags/reels/<reel-uuid>/labels
Authorization: Bearer <token>
Dust-Ctx-Org-Id: <org-uuid>
Dust-Ctx-Team-Id: <team-uuid>
Content-Type: multipart/form-data; boundary=<generated-by-http-library>
```

Multipart fields:

```text
position       = "1"
data           = <PNG crop bytes, filename and image/png MIME type>
humanReadable  = "<printed serial>"       # optional under API contract
qrValue        = "<canonical QR URL>"     # optional under API contract
options        = {"indexing":"none"}    # JSON-encoded form string
```

Indexing values are **`default` = identifiable** and **`none` = verify-only**. The UI labels `identifiable` and `verify-only` are not the wire values. Never silently rely on APID's identifiable default.

The API accepts image bytes/base64—not `imagePath`, `imageUrl`, an S3 URI or a whole stitcher `reel.json` as this request body. The worker reads the selected S3 crop and sends the bytes. clid's S3 support is client-side fetching, not an APID bulk S3 enrollment endpoint.

### 8.4 Recommended general path: keyed/named or multiple DUST identifiers

The stitcher already produces identifier names/descriptions, so implement this path, not only the shortcut:

1. For each selected DUST slot, read/validate its crop bytes and call:

   ```http
   POST /api/v1/tags/extract
   Content-Type: multipart/form-data; boundary=<generated>
   data = <crop bytes>
   ```

2. Checkpoint the returned fingerprint `id` per crop hash and destination context. Treat extraction as a server-side write; it is not an offline dry run.
3. Create the Label using the same Reel-label endpoint with form fields:

   ```text
   position    = "1"
   options     = {"indexing":"none"}
   identifiers = [
     {"tagType":"DUST","fingerprintId":"<returned-id>","name":"Large shield","description":"Primary DUST"},
     {"tagType":"TEXT","value":"<serial>","name":"Serial"},
     {"tagType":"QR","value":"<canonical QR URL>","name":"QR"}
   ]
   ```

   `options` and `identifiers` are JSON strings inside multipart. Include every approved DUST fingerprint and value member in **one** Label-create call.

4. Validate the returned outcome (`created` or `already_enrolled`), Reel UUID, position, live member IDs/types/counts/values and indexing before recording completion.

Fingerprint reuse/expiry semantics must be verified against deployed APID/scand. Reuse persisted IDs where supported; a rejected/stale fingerprint is not a reason to change the approved crop. Re-extraction, when necessary, remains tied to the same immutable image and plan.

The optional endpoint `/api/v1/composite-tags/{compositeTagId}/identifiers` can add members to an existing Label. Do not use it as the normal multi-crop strategy or silently change enrolled plans; repairs require their own approved operation.

### 8.5 Reconcile final state

Fetch Reel detail and compare **per position**, not only total counts:

- Intended Label identity and returned `compositeTagId`.
- Required live DUST members and recorded fingerprint/tag mapping.
- TEXT/QR values with the correct normalization semantics.
- Indexing mode, archive/transfer/bind status and unexpected members/warnings.
- Explicit exclusions or gaps, if the project allows them.

DUST values are not returned as ordinary public identifier text, so retain the enrollment receipt linking approved crop hash → fingerprint → returned identifier IDs. Do not claim that counting DUST members alone proves image identity.

The pilot stays in the same plan/Reel and is skipped during the remainder. It is not deleted/recreated or enrolled at a temporary position.

The clid 25 MiB image cap is a **client policy**, not a confirmed APID server limit. Adopt a conservative tested crop limit initially, and verify gateway/server/algorithm limits against the deployed environment. Supplied 640×480 PNG crops should be measured, not automatically recompressed.

## 9. Durability, retries and idempotency

### 9.1 No universal exactly-once promise

APID reconciles a repeated DUST when it already belongs to a compatible unbound, active Label on the same Team, Reel and explicit position. A different assignment is a conflict. Scan calls happen outside the final database membership transaction, so side effects can exist even when a client sees an error.

There is no generic idempotency-key handling in the inspected Reel/Label creation path. Do not add an `Idempotency-Key` header and assume the server implements it.

Use **at-least-once delivery with durable intent, conservative reconciliation and explicit conflicts**, not “every endpoint is idempotent.”

### 9.2 Enrollment row transaction boundaries

For each immutable plan row:

1. Claim the row with a lease/fencing token; permit only one active enrollment owner per destination Reel.
2. Persist intent and request digest **before** the network call.
3. Checkpoint each successful extraction response separately.
4. Persist Label-create intent with explicit position and fixed member set.
5. Send the request.
6. Validate and durably store the response/IDs.
7. If the outcome is ambiguous, set `reconciling` and inspect APID before attempting any changed/new operation.

A recovered worker cannot assume an expired lease means the old request did not commit. If remote state cannot prove compatibility, pause for resolution. Never treat any 409 as “already enrolled.”

Digest inputs include environment, org, Team, Reel UUID, position, selected artifact versions/hashes, values, slot metadata and indexing. Signed URL query strings and mutable UI fields are excluded.

### 9.3 Retry policy

| Failure | Action |
|---|---|
| S3 timeout/5xx | Retry the same object/digest with bounded backoff and a refreshed URL if needed |
| Network loss during APID mutation | Mark uncertain; reconcile, then retry the same intent if safe |
| 429 | Respect `Retry-After`; reduce concurrency |
| 401 | One synchronized refresh; then retry/reconcile or pause authentication |
| 403 / feature disabled | Pause; operator/admin action, not repeated token exchanges |
| Invalid image/QR/position/known quality failure | Permanent row review until inputs are explicitly corrected |
| APID/scan 5xx | Classify machine code; transient outages may retry, extraction/content failures need review |
| DUST/position/Vlink conflict | Reconcile and surface conflict; never automatically overwrite/move |
| Worker crash/OOM | Preserve completed rows; reconcile ambiguous rows; resume with original context |

Start with low enrollment concurrency (e.g. 2–4 labels) and a separate extraction limit; tune against scan-service capacity and actual 429/latency measurements. No unbounded fan-out per Reel or across stations.

Pause stops scheduling new requests; in-flight writes may finish. Cancel does not undo completed APID enrollments. The UI must explain both facts.

### 9.4 Ambiguous Collection/Reel creation

Local unique job keys cannot make remote non-idempotent creation exactly once. If a create response is lost, a repeated name is not proof of identity.

- Persist creation intent and search/review possible outcomes; never blindly recreate.
- Prefer preselected server IDs for the initial pilot.
- Before unattended creation is enabled, add server-supported scoped idempotency keys or an equivalent unique client-operation reference to Collection/Reel creation, with replay/conflict tests.

This enhancement is not necessary for a supervised first enrollment into existing Reels, but is important for production automation.

## 10. New workflow service API

**Proposed endpoints below do not exist in the reviewed repos.** Use a separate `/pipeline/v1` namespace; it can live beside the upload service with appropriate backend routing.

| Endpoint | Contract |
|---|---|
| `GET /pipeline/v1/projects` | Authorized projects, destination context, approved profile versions |
| `POST /pipeline/v1/runs` | Register station capture or approved S3 import; client operation ID |
| `GET /pipeline/v1/runs` | Paginated authorized local/cloud run list |
| `POST /pipeline/v1/runs/{id}/upload-batches` | Allocate safe object keys and checksum-aware signed PUTs |
| `POST /pipeline/v1/runs/{id}/complete-upload` | Verify sealed inventory and commit input revision |
| `POST /pipeline/v1/runs/{id}/processing-attempts` | Enqueue one pinned profile/image version; return attempt ID |
| `GET /pipeline/v1/attempts/{id}` | Durable phase/status/counts/findings |
| `POST /pipeline/v1/attempts/{id}/cancel` | Request safe cancellation |
| `GET /pipeline/v1/attempts/{id}/labels` | Paginated candidates and thumbnails; filters |
| `POST /pipeline/v1/attempts/{id}/reviews` | Reviewed decisions with expected revision to prevent stale overwrites |
| `POST /pipeline/v1/enrollment-plans` | Freeze selected results, destination, positions, indexing and approval digest |
| `POST /pipeline/v1/enrollment-plans/{id}/start` | Pilot or approved remainder; idempotent command ID |
| `POST /pipeline/v1/enrollment-plans/{id}/pause` | Stop scheduling new enrollment rows |
| `POST /pipeline/v1/enrollment-plans/{id}/resume` | Recover the same plan, not create a new one |
| `GET /pipeline/v1/enrollment-plans/{id}` | Per-row outcomes, conflict details and reconciliation status |
| `GET /pipeline/v1/events?after=<sequence>` | Pollable/resumable events, SSE transport optional |
| `GET /pipeline/v1/artifacts/{id}/download` | Authorized short-lived read URL for a registered artifact |

Creation commands return durable IDs/202 where asynchronous. Enforce command IDs in PostgreSQL; duplicate requests return the same resource. Use revision/ETag checks for reviews and approval.

The backend resolves project→bucket/prefix/APID org/Team credentials. The desktop cannot choose arbitrary storage URLs, image tags, shell arguments or privileged destination contexts.

## 11. State and persistence

### 11.1 Source of truth

| Data | Authority |
|---|---|
| Unuploaded capture bytes + upload journal | Station filesystem/SQLite |
| Sealed raw bytes and published algorithm artifacts | S3 immutable versions/digests |
| Job lifecycle, review, approval, enrollment intent/receipts | Workflow PostgreSQL |
| Actual Label/Reel membership | APID |
| Desktop cloud status/thumbnails | Rebuildable cache |

Avoid one mutable `labels.json` being edited simultaneously by the algorithm, UI and enrollment worker.

### 11.2 Minimal tables

- `runs`: project, station, operator, storage locator, capture digest, capture mode/settings, loss counters, revision.
- `processing_attempts`: run/input revision, profile/assets/image digests, Job UID, phase, heartbeat, lease, output inventory, error.
- `label_candidates`: attempt, candidate ID, scan index/anchor, identity/provenance, quality findings.
- `artifacts`: owner context, key/version/hash/size/type, label candidate and crop slot.
- `reviews`: candidate/revision, decision, reviewer, reason, timestamp.
- `enrollment_plans`: approval digest, target environment/org/Team/Collection/Reel, position mapping, indexing, state.
- `enrollment_rows`: frozen row digest/position, checkpointed extraction IDs, returned Label/identifier IDs, status, attempts, classified error.
- `jobs/outbox/events`: durable commands, leases/fencing, observable state changes and audit trail.

Use unique constraints for command IDs, `(plan_id, position)`, and active destination-Reel ownership. Enforce referential integrity and row-state transitions in transactions. Keep images out of the database.

### 11.3 State machines

```text
Capture:
  capturing → flushing → sealed → uploading → raw_verified
                          ↘ needs_attention / recoverable_error

Processing attempt:
  queued → staging → matching → extracting → rendering → publishing
                                                    → awaiting_review
                 ↘ failed / cancelled / needs_attention

Enrollment plan:
  draft → approved → pilot_running → pilot_verified → enrolling
       → reconciling → completed
       ↘ paused / partial / needs_attention / cancelled

Enrollment row:
  pending → extracting → extracted → submitting → verified
                                 ↘ reconciling → verified / conflict / retryable
```

Treat these as linked stage states, not one linear status flag that cannot represent “upload paused while capture is usable” or “stitch finished but quality failed.” Event notifications are hints; DB state and artifact verification remain authoritative.

## 12. Desktop implementation and UX

### Screens

1. **Capture:** new web controls over existing camera/capture operations, with project/run header and durable loss counters. Reproduce approved exposure/gain, preview, mode, start/stop and simulator behavior; do not retune the hardware recipes.
2. **Runs:** unified local/cloud list; stage badges, profile, count, owner, last update; resume/import actions.
3. **Run detail:** capture inventory, S3 verification, attempt history, direction and stage timeline.
4. **Review:** virtualized label table/grid, exception filters, full-resolution crop and raw-neighbor drill-down, selected identifier slots, serial/QR provenance, position mapping.
5. **Enroll:** destination UUID/name, indexing explanation, required/approved counts, validate/pilot/start/pause/resume, per-row classified failures.
6. **Results:** authoritative reconciliation table, explicit gaps/conflicts, CSV/JSON export and APID link.
7. **Settings:** approved project/profile selection, upload limits, connection status, local cache/retention. Restrict algorithm tuning and production settings to authorized roles.

Use legible status text as well as color. Keep irreversible actions explicit. A single “Process and enroll everything” button is not appropriate for the first release.

### Code organization

Proposed desktop layout in this repository; the Workflow backend's repository remains an S00 decision. These are future paths, not existing/scaffolded modules:

```text
apps/desktop/
  src/                           # web screens and typed native bridge
  src-tauri/
    src/commands/                # narrow validated application commands
    src/capture/                 # helper supervisor + protocol + preview handles
    src/auth/                    # browser/PKCE + OS credential storage
    src/workflow/                # authenticated HTTP, polling, reconnect
    src/storage/                 # SQLite, scoped files, verified uploads
    capabilities/                # permissions for the bundled main webview
    binaries/                    # generated target-specific helper, not Git data
packages/contracts/              # schemas + TypeScript/Rust/Python golden fixtures
```

Build the capture helper from a pinned, approved `labeltron-two` working branch/package, not by importing `reference/` at runtime or copying that tree into Git. Proposed upstream additions are `src/labeltron/headless/` for the protocol wrapper and `src/labeltron/capture/seal.py` for sealing. The dependency/distribution arrangement and upstream owner must be confirmed in S00. Reuse `BurstRunner`, `RunRequest`, capture events, `CameraSystem`/`CameraDevice`, `SimulatedCameraSystem`, runtime/preflight and settings/layout code. Keep the existing CLI/Qt app working as regression clients of the shared core.

Keep native HTTP, hashing, disk access and subprocess reads off the renderer and GUI event loops. Only the Rust host writes the local upload/command SQLite journal. The helper writes capture files and atomically publishes manifests; the host verifies them before registration/upload. Browser development uses fake native adapters, not an exposed camera-control HTTP server.

### Native/helper protocol and lifecycle

- Rust starts only the bundled, pinned helper executable with fixed arguments; do not expose generic shell/execute or arbitrary filesystem commands to JavaScript. Validate custom native commands and their window permissions explicitly; Tauri plugin scopes do not sandbox Rust code.
- Use bounded versioned JSON Lines over inherited stdin/stdout for commands, responses and status events; reserve stderr for redacted logs and drain both streams concurrently. Include protocol/helper/core version handshake, request IDs, helper-session generation, run IDs, stable error codes and timeout/cancellation semantics. Reject incompatible versions and oversized/malformed messages.
- Expose a small operation set: preflight, list/connect/disconnect camera, validated settings, preview start/stop, capture start/status/stop and helper shutdown. Adapt typed core callbacks; do not parse `labeltron-cli` log text or serialize `FrameKept.image` arrays into JSON.
- Keep raw frames on disk. Rate-limit/downsample a separate bounded latest-frame preview cache; the renderer receives opaque scoped image handles, not arbitrary file paths. Drop preview work under pressure, never raw capture frames to satisfy UI throughput. Preview is not enrollment evidence.
- Use one capture owner per camera/run. Persist a start intent before sending; a lost start response is unknown until status/session/run inventory is reconciled. Never blindly replay start after timeout or helper restart. A renderer reload reattaches to native state without spawning a second helper.
- A Stop acknowledgement is not a seal: wait for acquisition stop, writer/QR drain, camera cleanup and validated manifest publication. Normal application close offers cancel or stop-and-flush before exit. Host loss/pipe EOF requests safe helper shutdown; bounded forced termination is a last resort and leaves the run unsealed/needs-attention. Verify Windows process-tree containment; never kill an unrelated camera process.
- Cloud-only review works when the helper, camera or Vimba runtime is unavailable. Capture/upload stops or checkpoints when the native host exits; already approved cloud processing/enrollment continues. Do not claim UI closure implies local acquisition safely completed.

Lazy-load thumbnails, fetch full crops on demand and cap caches. Current self-contained stitcher reports can reach gigabytes according to its documentation; do not use an embedded giant HTML report as the primary review screen. Keep it as an optional diagnostic artifact; generate smaller/link-based reports when needed.

### Windows packaging

Use a Tauri Windows bundle (NSIS proposed; MSI only if required), with a target-specific PyInstaller-built **headless capture** executable via `bundle.externalBin` and any required runtime resources. Reuse the old packaging's dependency/licence and preflight findings, not its Inno installer as the Tauri entry point. Build and smoke-test on Windows; do not assume a macOS build proves camera DLL loading or child shutdown.

Operators should not need Python, Node or Rust installed. Account for WebView2 bootstrap/offline provisioning and patch ownership, plus Vimba transport-layer prerequisites/licensing separately. Preserve existing YAML/capture paths through explicit import/migration; never silently delete the old app's data or permit simultaneous camera ownership. Sign the host/helper/installer as applicable, verify bundled helper/core/protocol compatibility and test upgrades/rollback with populated SQLite. Cloud-only review must survive missing camera/Vimba. No clid binary or local stitcher stack is bundled.

## 13. Required hardening found during review

These are concrete integration risks, not reasons to replace the underlying code.

| Priority | Finding | Required treatment |
|---|---|---|
| P0 | `docker/entrypoint.sh::run_logged` loses the algorithm's exit code: the final `echo` makes the piped group succeed | Capture and return the child status; preserve logging errors separately; regression tests. **Reproduced: stub exit 42 → wrapper exit 0.** |
| P0 | Missing crops intentionally do not make `stitchin-complete` fail; CSV completeness can precede crop-render failures | Separate execution success from quality; validate actual keyed manifest and crop files |
| P0 | `cloud/sync.py` skips when local size is **less than or equal to** remote size | Replace with exact verified-object identity; same-size/different bytes and larger stale objects must not pass |
| P0 | Capture/sync have no verified whole-run completion contract | Atomic files, real flush barrier, sealed manifest, backend verification and enqueue-once |
| P0 | Matching checkpoint fingerprint includes frame **names**, not raw bytes; composites reuse an existing decodable filename | Bind caches to input content + algorithm/profile/model digests; never reuse approved workspace in place |
| P0 | Crop paths use sanitized serials, so duplicates/sanitization collisions can target the same output file | Add collision detection before rendering and preferably label-ID-based internal output names; importer rejects duplicates |
| P1 | Resume ignores a truncated last JSONL line but append does not first truncate that tail | Repair tail to last complete record before append; test crash→resume→crash→resume |
| P1 | Cloud auth returns only ID token/email; no refresh lifecycle; all upload URLs are minted before the batch uploads | Secure refresh/re-login lifecycle; just-in-time URL windows; cancellation and bounded retry |
| P1 | Upload folder root and stitcher `scans/` prefix conventions differ | Store actual source locator; explicit staging adapter and import tests |
| P1 | Stitcher loader is nonrecursive; camera supports nested bursts | Qualify stream-first and add a deliberate burst adapter, not arbitrary recursive globbing |
| P1 | Resume clears crops; ephemeral Job storage and final uploads are not crash durability | Immutable published attempts; validated checkpoint restore/retention and central logs |
| P1 | Upload-stack source uses bucket `RemovalPolicy.DESTROY` + `autoDeleteObjects` | Verify live infrastructure; set retention/backup protections before production data depends on it |

The source also has macOS-specific Cargo configuration and native OpenCV dependencies. A Windows-local stitcher would require a separate build/packaging qualification. The Dockerfile does not copy the local `.cargo/config.toml`, so do not infer that its release image inherits `target-cpu=native`; inspect actual image build settings and pin a compatible architecture.

See [SOURCES.md](SOURCES.md) for precise locations and the distinction between confirmed behavior and untested deployment assumptions.

## 14. Security, deployment and operations

### Identity boundaries

- **Operator:** retain the Google system-browser/PKCE flow for upload/workflow access, implemented in the native host rather than depending on the old Qt cloud panel. Validate callback state/nonce and approved redirect/origin, handle expiry/re-login, and validate tokens/project permissions server-side. An email/domain alone is not sufficient authorization to enroll into an arbitrary Team.
- **Enrollment worker:** team-scoped APID Service Account; key in a cloud secret manager, short-lived APID token in worker memory. Audit both initiating operator and executing service principal.
- **S3/stitcher:** workload IAM role with read access to approved inputs/assets and write access to that job's output scope. No APID enrollment secret in the algorithm container.
- **Desktop:** no cloud master keys. Rust owns operator tokens and Windows Credential Manager access; do not put bearer/refresh tokens in JavaScript storage or pass them to the capture helper. Desktop OAuth client secrets cannot be treated as confidential application secrets.
- **Webview/native boundary:** bundle UI assets for offline use, enforce CSP and explicit command capabilities for the trusted window, and deny remote content native access. Do not render diagnostic HTML in a privileged webview. Native handlers must enforce paths, command state and allowlisted API/S3 destinations themselves; CSP/capabilities are not a substitute for validation.

Google upload login and APID identity are separate trust domains; do not forward a Google ID token to APID and expect it to work. A later AuthD operator login can unify the desktop experience only after upload/workflow audience and token-exchange policy are explicitly designed.

### Storage and data safety

- Private encrypted buckets, TLS, scoped signed URLs, no secrets/presigned query strings in logs or exported plans.
- Project checks on every run/artifact/approval operation; prevent confused-deputy access through backend service credentials.
- Reject path traversal, absolute paths, symlinks escaping local root, arbitrary S3 buckets and external URLs. Validate decoded image dimensions/pixel counts and bounded JSON/CSV sizes.
- Do not execute operator-provided YAML paths or shell arguments. Asset bundles are vetted artifacts, not an arbitrary-code extension mechanism.
- Escape serial/QR/log content in UI/reports; neutralize spreadsheet formulas in CSV exports.
- Pin worker images/dependencies; scan images and generate licence/SBOM records. Keep original capture/crop bytes.

### Capacity and monitoring

The stitcher docs suggest 8 CPU, 16 GiB RAM and 150 GiB staging storage per full Reel, with roughly 11,000 frames and tens of GB of raw data. They report about 12 minutes locally versus about 20 minutes in the cluster guide; **these are source-reported examples, not benchmark results from this review or an SLA**.

Measure upload bandwidth first: 44 GB at 100 Mb/s is roughly 59 minutes before overhead, potentially longer than stitching. Backend placement near S3 removes an unnecessary raw-data round trip but does not remove the initial station upload.

Bound fleet-wide concurrent Jobs and enrollment extraction, record p50/p95 phase latency, raw bytes, memory, retries, queue age and API failures. Alert on stuck leases, partial uploads, repeated authorization failure, capture losses and reconciliation mismatches.

Keep database backups/PITR and protected S3 retention. Define separate retention for raw evidence, approved crops/receipts, optional composites/debug HTML and scratch PVCs. Local raw cleanup is allowed only after verified cloud durability and the agreed retention policy—not immediately after pressing Sync.

## 15. Alternatives and where they fit

| Architecture | Benefits | Costs / suitable use |
|---|---|---|
| **Selected shell; recommended deployment: Tauri/web UI + Python capture helper + cloud stitching/enrollment** | Preserves capture core; cloud jobs survive desktop exit; centralized enrollment credentials/leases | New UI/IPC/helper packaging plus Workflow API/database and operated compute |
| Tauri/web UI + Python capture helper + cloud stitching + native desktop APID worker | No cloud enrollment worker initially; crop downloads are small | Station must remain online; native APID auth/durable row recovery; prevent duplicate operators |
| Tauri/web UI + Python capture helper + local Rust stitcher | Offline processing possible | Separate Windows OpenCV/stitcher qualification, resource contention; outside initial scope |
| Extend existing Qt application | Smaller presentation/packaging change | Earlier recommendation, superseded by the user's Tauri choice; retain as a regression/reference client only |

If choosing desktop-direct enrollment, port the same verified HTTP contract into a native Rust worker, use Windows credential storage and a durable local plan/row journal, and keep an exclusive backend Reel lease. Do not put APID credentials or enrollment logic in the renderer or camera helper. Do not transfer responsibility by shipping clid. Online enrollment is still impossible while APID is unreachable.

Do not build cloud and local algorithm execution simultaneously for v1. Define the runner interface now and deliver the cloud adapter first; add local execution only for a proven requirement.

## 16. Delivery milestones and acceptance criteria

The actionable backlog is [IMPLEMENTATION_SLICES.md](IMPLEMENTATION_SLICES.md): 25 parent slices, with explicit Tauri/helper child gates, dependencies, TODO checklists and exclusions. These milestones describe release outcomes; the slice plan defines execution order, including an existing-S3 path and parallel capture/upload work. No implementation has started.

The earlier **8–12 week estimate assumed Qt UI/installer reuse and is withdrawn**. Re-estimate after the early Tauri shell and packaged-helper proofs; new UI/IPC work, native packaging, infrastructure access and algorithm qualification are not yet sized.

### M0 — Verify contracts and de-risk

- Prove the Tauri shell/fake bridge and Windows bundle early after S01; then qualify the packaged Python simulator/helper protocol and safe lifecycle before hardware integration.
- Obtain approved representative raw runs and ground-truth label associations without modifying protected reference manifests.
- Confirm actual deployed upload prefix, bucket, cloud compute, APID version/entitlements and scan-service availability.
- Confirm profile, expected DUST slots, serial authority and Reel-position semantics.
- Fix/test masked process exit; run one controlled cloud stitch and one direct-API label enrollment into a disposable nonproduction Reel.
- Test the named/multi-DUST path even if the first product exports only one DUST.
- Measure upload time, compute memory/runtime and APID extraction/enrollment latency.

**Exit:** known-good input → verified correct crop/QR/serial → direct APID enrollment → reported identifiers. No UI rewrite required to prove this.

### M1 — Durable ingest and run registry

- Desktop run IDs, flush/seal protocol, local upload journal and checksum-aware uploads.
- Workflow auth/project mapping, PostgreSQL schema, outbox, existing-S3 import and state API.
- Upgrade storage protection and prefix mapping.

**Exit:** interrupt capture/upload/app; reopen and recover without silently accepting missing or different bytes. Duplicate completion requests produce one run/processing intent.

### M2 — Cloud processing orchestration

- Pinned Job launch/reconciliation, staging validation, cancellation and resource caps.
- Structured result importer/progress, immutable artifact publishing and quality classifications.
- Safe exact-version checkpoint recovery; fix cache/path collision defects.

**Exit:** duplicate requests, failed child process, OOM and partial output upload cannot create a false successful result or duplicate active job.

### M3 — Review and approval

- Run history, virtualized candidate grid, full-resolution/source drill-down.
- Identity/position/required-slot checks, warning disposition and immutable approval digest.
- Profile qualification tests, including first/last labels and reverse feed.

**Exit:** a bad association or missing required crop is visibly blocked; reprocessing/editing cannot reuse stale approval.

### M4 — Direct APID enrollment

- Typed auth/context, Collection/Reel selection, extraction and Label-create adapter.
- Durable row/extraction checkpoints, pilot/remainder, bounded concurrency, pause/resume and conflict UI.
- Per-position reconciliation and receipts/reports.
- Add remote create idempotency before unattended Collection/Reel setup, or retain supervised existing-Reel selection.

**Exit:** simulate a lost response after a server commit; resume resolves the original Label without duplication or changing positions. Existing compatible/conflicting enrollment scenarios are covered.

### M5 — Production hardening and pilot

- Web capture-control parity and integrated journey; signed Tauri/helper installer, WebView2/Vimba preflight, supported Windows/camera tests, upgrades/rollback and credential renewal.
- Fleet/job caps, monitoring, backups/retention, security/authorization tests and runbook.
- Complete a full real Reel, independently verify association quality and reconcile intended versus actual APID state.

**Exit:** a production-sized, reviewed run completes end-to-end, recovery drills pass, and operators can resolve a failure without command-line tools.

## 17. Test and acceptance matrix

| Layer | Essential tests |
|---|---|
| Capture | Existing simulator/golden core behavior retained; web-control parity; Unicode Windows paths; disk full; writer timeout; crash before seal; dropped frames |
| Tauri/helper | Fake native adapter; protocol mismatch/malformed output; stdout/stderr backpressure; preview saturation; duplicate start; renderer reload; helper/native crash; EOF/stop/flush; camera owner exclusion |
| Upload | Same-size changed object, larger stale remote object, expired URL, token expiry, >500 files, pagination, partial batch, restart, duplicate completion, attempted namespace escape |
| Stitch runner | Child exit propagation; missing crop with exit 0; wrong orientation/profile; native crash/OOM; result upload failure; zero labels |
| Checkpoints | Same filenames/different bytes, model/profile/image change, truncated JSONL repaired before append, no retained PVC, missing composite, repeated resume |
| Identity/QC | Neighbor QR misassociation, missing endpoint labels, duplicate serial/QR, sanitized path collision, ambiguous direction, print-manifest gap versus actual physical order |
| Contracts | Golden schemaVersion-2 keyed manifests; optional/null fields; multiple DUST slots; multipart names/JSON strings; indexing wire values; unknown fields |
| APID integration | Auth/Team mismatch, returned member mismatch, pilot/remainder, occupied position, already-enrolled DUST same/different Reel, transferred/bound Label, 429, extraction failure |
| Recovery | Crash before send / after remote commit / before local response checkpoint; two workers; stale lease; server-created Reel response lost; stopped UI while cloud enrollment continues |
| Security/UI | Restricted custom commands/capabilities, CSP and remote-origin rejection; no generic shell/path/URL proxy; no tokens in renderer/helper; unauthorized project/artifact access, forged destination, secret redaction, responsive virtualized UI, safe HTML/CSV |
| Release | Clean Windows without Python/Node/Rust, WebView2 missing/present/offline provisioning, packaged helper/host version mismatch, Vimba missing/present, no camera review mode, old-app data import, credential expiry, schema migration and nondestructive rollback |

Algorithm correctness acceptance should be based on verified label↔QR↔DUST correspondence and false-association rate on a representative set, not a made-up aggregate confidence or a green process exit. Throughput targets follow M0 measurements.

## 18. Decisions to settle before implementation

1. Tauri + web UI with retained Python capture is selected. Confirm React/TypeScript/Vite, the helper's upstream packaging/distribution home, and supported capture versus cloud-only station modes; Tauri does not itself qualify camera support.
2. Is the supplied EKS/ECR setup deployed and owned, or should the same container run on Batch/ECS?
3. Which label profiles ship first, and should the long label enroll one DUST crop or all three?
4. Who supplies/version-controls the approved serial manifest? Is Vlink alias lookup an approved substitute?
5. What exactly does APID Reel position mean for reversed runs, partial scans and rescans?
6. Are required-label gaps allowed? Recommended v1: no silent skipping; whole-plan approval waits for resolution.
7. Verify-only or identifiable enrollment? Make the project policy explicit; do not inherit APID's default by accident.
8. Required workload: runs/day, labels/run, raw size, station bandwidth, simultaneous stations and acceptable turnaround?
9. Can enrollment run under an audited Team-scoped worker Service Account, or must the individual operator's APID identity perform writes?
10. Production retention, code-signing ownership, supported Windows versions and recovery/support responsibilities?

### First code slice and early proofs

After S00 acceptance, start **S01: executable contracts, canonical identities/digests and safe fake adapters**, including the native/Python helper boundary. Then prove the Tauri shell in S10a and the packaged capture helper in S17a alongside engine/backend work.

S04 remains the early opt-in direct-APID proof after its inherited gates: a known approved S3 run -> pinned stitcher -> verified crop -> one nonproduction Label. Do not confuse that demonstration with the first code ticket or full product approval. Build the new presentation layer without rewriting camera recipes, replacing the stitcher, or shipping clid.
