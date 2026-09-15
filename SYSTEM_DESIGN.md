# Labeltron Enrollment - System Design

**Tauri + web UI with retained Python capture and backend-first, script-before-UI delivery selected. The new Workflow backend is not implemented; cloud deployment and remaining pilot decisions are still proposals.** See [S00](plans/S00-pilot-scope.md).

Detailed implementation plan: [DESKTOP_APP_PLAN.md](DESKTOP_APP_PLAN.md).
Slice-by-slice TODOs and dependencies: [IMPLEMENTATION_SLICES.md](IMPLEMENTATION_SLICES.md).
Verified source contracts and findings: [SOURCES.md](SOURCES.md).

## 1. Core decision

One desktop experience, with different responsibilities on the station and in the cloud:

```text
     CAPTURE          ARCHIVE         PROCESS          APPROVE          ENROLL
        |                |               |                |                |
        v                v               v                v                v
   Windows app -------> S3 -------> Rust stitcher ---> Desktop review ---> APID
        |                                |                |                ^
        |                                v                v                |
        +----------------------> Workflow service + durable state --------+
                                    cloud orchestration
```

- Build a Tauri v2 shell with a bundled web UI (React/TypeScript/Vite proposed); replace the Qt presentation layer.
- Reuse the Python camera/capture core in a supervised helper; preserve camera recipes and simulator/golden behavior. Native Rust owns credentials, IPC, cloud HTTP and the upload journal.
- Run the existing Rust/OpenCV stitcher as a cloud job, near S3.
- Add a workflow backend for jobs, review, approval, progress and recovery.
- Its enrollment worker calls APID directly. No clid executable or CLI parsing.
- Keep capture available offline. Verified cloud jobs continue if the desktop closes.

**"Desktop app" describes the operator experience, not where every computation must run.**
If enrollment requests must originate on Windows, the alternative is in section 12.

Legend:

```text
[E] Existing implementation       [H] Existing, but needs hardening
[N] New component/contract        --> Control or metadata
==> Image/artifact bytes          <-> Request/response or state synchronization
```

## 2. End-to-end deployment diagram

```text
WINDOWS STATION

 [N] Tauri v2 + bundled web UI
     Capture / Runs / Review / Enroll / Results
                  | typed commands / bounded events
                  v
 [N] Native Rust host
     - scoped commands + helper supervisor
     - operator credentials + cloud HTTP
     - SQLite upload journal + command IDs + rebuildable cloud cache
                  | versioned stdio                 ^ sealed inventory
                  v                                 |
 [N] Python capture helper -------------------------+
     [E] camera/simulator + acquisition/QR
     [H] writer -> atomic raw frames -> flush -> [N] sealed manifest
          ^ USB frames from [E] Alvium + gap sensor

 Native host: verified upload workers + authorized preview/artifact I/O
                  |                        |
                  | HTTPS metadata         | signed PUTs: raw image bytes
                  |                        | (no AWS keys on the station)
                  v                        v
+-------------------------------- CLOUD BOUNDARY ---------------------------------+
|                                                                                 |
| +-------------------------+      +--------------------------------------------+ |
| | [N] Workflow API        |      | [H] Private S3                              | |
| |                         |      |                                            | |
| | - authenticate/authorize|      | raw: sealed capture versions                | |
| | - register/import runs  |      | assets: pinned profiles/templates/models   | |
| | - allocate upload keys  |      | outputs: crops, JSON, QC, thumbnails        | |
| | - verify upload commit  |      | diagnostics: logs/checkpoints/composites   | |
| | - submit processing    |      | receipts: approved plans/final reports      | |
| | - record review        |      +-------|------------------|-----------------+ |
| | - approve/start/pause   |              |                  ^                   |
| | - progress/artifact URLs|              | raw + assets     | output bytes      |
| +------------|------------+              v                  |                   |
|              |                  +--------------------------------------------+ |
|              v                  | [E/H] Rust/OpenCV stitcher Job              | |
| +-------------------------+     |                                            | |
| | [N] PostgreSQL          |     | stage inputs -> match -> stitch -> QR      | |
| |                         |     | -> regroup physical labels -> render crops | |
| | runs / attempts         |     | -> publish algorithm artifacts             | |
| | candidates / artifacts  |     |                                            | |
| | reviews / approvals     |     | scratch volume; pinned container digest    | |
| | plans / enrollment rows |     +--------------------^-----------------------+ |
| | outbox / jobs / events  |                          | launch/watch/cancel      |
| +-------^------------^----+                          |                          |
|         |            |                 +------------|------------------------+ |
|         |            +---------------->| [N] Scheduler + result importer     | |
|         |                              | leases, job recovery, output QC      | |
|         |                              +-------------------------------------+ |
|         |                                                                      |
|         |               +-----------------------------------------------+      |
|         +-------------->| [N] Enrollment worker                         |      |
|                         |                                               |      |
|              S3 crops ==>| verify approved hashes; extract each slot     |      |
|                         | create Label; checkpoint; reconcile            |      |
|                         +-----------------------|-----------------------+      |
|                                                 | direct HTTP                  |
+-------------------------------------------------|------------------------------+
                                                  v
+---------------------------- EXISTING DUST SERVICES ------------------------------+
|                                                                                 |
| [E] AuthD                       [E] APID                                          |
|     service-account key             Collections / Reels / Labels                  |
|            |                        image extraction + enrollment APIs           |
|            +--> short-lived JWT --> request auth + org/Team context               |
|                                                   |                             |
|                                                   v                             |
|                                     Existing DUST scan/algorithm services        |
|                                     (called by APID, not by the desktop)          |
+---------------------------------------------------------------------------------+
```

The API, scheduler, result importer and enrollment worker are roles in **one new backend codebase**, not four independently designed products. They are not implemented yet. Existing APID/AuthD and the native stitcher provide reviewed component contracts, not a ready end-to-end Workflow service; target-environment operation remains unvalidated.

Use the supplied EKS Job model if that infrastructure is operated already. Otherwise run the same container on Batch/ECS; do not build a Kubernetes platform merely to satisfy this diagram.

### 2.1 Tauri-to-capture boundary

```text
 Bundled web UI                Native Rust host                 Python helper
       |                            |                               |
       |--- typed invoke ---------->|--- fixed bundled executable ->|
       |                            |<-- version/session handshake --|
       |                            |--- request ID + command ------>|
       |                            |<-- response + bounded events --|
       |<-- state / preview handle -|                               |
       |                            |<-- redacted stderr logs -------|
       |                            |                               |
       |--- stop ------------------>|--- stop request -------------->|
       |<-- stopping/flushing ------|<-- drain writer + QR ----------|
       |                            |<-- camera cleanup + seal ------|
       |<-- sealed or needs_action -|--- verify manifest + journal    |
       |                            |                               |
       | renderer reload            | helper/native crash or EOF    |
       +--> reattach to host state  +--> unknown/unsealed; reconcile |
                                        never blindly restart run
```

- Use bounded versioned JSON Lines over inherited pipes, not a public/local camera-control HTTP server. Parse typed messages, not CLI log text; concurrently drain stdout/stderr and reject incompatible versions.
- Keep raw pixels on disk. The helper emits rate-limited preview metadata for a bounded preview cache; Rust validates opaque image handles and serves only scoped bytes. Dropping preview is allowed; affecting saved raw frames is not.
- One helper owns a camera/run. Persist start intent, reconcile lost acknowledgements, and prevent duplicate starts after reload/restart. `RunFinished` or exit 0 alone is not proof of a sealed capture.
- Normal app close stops and flushes local capture before exit; EOF/host death triggers bounded fail-safe shutdown. Forced termination leaves an unsealed run for recovery, not a success. Cloud processing/enrollment continues independently.
- No generic shell/file/URL proxy or tokens in the renderer, and no cloud credentials in the helper. A capture helper is not a stitcher or APID worker.
- Bundle the helper for the Windows target with Tauri; qualify WebView2, Python native DLLs, Vimba licensing and process-tree cleanup. Missing hardware/helper must not prevent cloud-only review. Details: [desktop implementation](DESKTOP_APP_PLAN.md#12-desktop-implementation-and-ux).

## 3. Control plane versus data plane

Large images do not pass through the workflow API or its database.

```text
CONTROL PLANE: small authenticated requests and durable state

 Bundled web UI
    | typed native commands; no bearer tokens
    v
 Tauri native host
    |
    +--> register run / request upload batch / commit sealed inventory
    +--> start processing / inspect progress / request retry or cancel
    +--> review candidate / approve exact revision / start enrollment
    +--> fetch row outcomes / final reconciliation
    |
    v
 Workflow API <--> PostgreSQL <--> Scheduler / enrollment worker
                                      |
                                      +--> Cloud Job API
                                      +--> APID HTTP endpoints

DATA PLANE: immutable bytes and content digests

 Local raw frames ===== signed PUT =====> S3 raw object versions
                                                ||
                                           verified GET
                                                ||
                                                vv
                                      Stitcher scratch volume
                                                ||
                                          publish artifacts
                                                ||
                                                vv
 Desktop previews <===== signed GET ===== S3 crops + thumbnails
                                                ||
                                          verified crop GET
                                                ||
                                                vv
                                        Enrollment worker
                                                ||
                                        multipart image bytes
                                                ||
                                                vv
                                               APID
```

The Python helper owns capture files/sealing; the Rust host streams sealed files and owns the SQLite transfer journal. Full raw frames never cross the renderer/JSON control path, and Python is not a second uploader.

Critical distinction: APID's Label-create route does not consume a whole S3 Reel manifest. The worker resolves the manifest, fetches the selected crop bytes and sends APID's supported multipart request.

## 4. Capture-to-S3 commit protocol

A folder existing in S3 does not mean capture is complete. The helper writes raw files/manifests; the Rust host performs the local journal/upload steps below. A stop response is not a seal acknowledgement.

```text
 CAMERA           CAPTURE HELPER         LOCAL STORE          WORKFLOW API       S3
   |                    |                     |                    |             |
   |--- frame --------->|                     |                    |             |
   |                    |-- encode bytes      |                    |             |
   |                    |-- temp file         |                    |             |
   |                    |-- atomic rename --->| raw frame          |             |
   |                    |                     |                    |             |
   |--- stop ---------->|                     |                    |             |
   |                    |-- drain queues      |                    |             |
   |                    |-- confirm stopped   |                    |             |
   |                    |-- write inventory ->| capture manifest   |             |
   |                    |                     | + hash + counters  |             |
   |                    |                     |                    |             |
   |                    |                     |-- register run --->|             |
   |                    |                     |<-- run ID ---------|             |
   |                    |                     |-- request URLs --->|             |
   |                    |                     |<-- signed batch ---|             |
   |                    |                     |                    |             |
   |                    |                     |==== frame bytes =================>|
   |                    |                     |<=== checksum/version receipt ====|
   |                    |                     |-- journal success  |             |
   |                    |                     |     repeat/resume  |             |
   |                    |                     |                    |             |
   |                    |                     |-- complete-upload >|             |
   |                    |                     |                    |-- verify -->|
   |                    |                     |                    |<-- status ---|
   |                    |                     |                    |             |
   |                    |                     |              [all inputs match?] |
   |                    |                     |                 /       \       |
   |                    |                     |               NO        YES      |
   |                    |                     |               |          |       |
   |                    |                     |          upload error    |       |
   |                    |                     |                          v       |
   |                    |                     |               raw_verified +     |
   |                    |                     |               outbox in one TX   |
   |                    |                     |                          |       |
   |                    |                     |                          v       |
   |                    |                     |                  processing job  |
```

Required safeguards:

- The signed upload/checksum/version protocol is new work; current sync checks sizes only.
- Verify exact bytes, not `local size <= remote size` and not an assumed ETag checksum.
- Use pinned object versions or immutable/write-once keys. Late outstanding upload URLs must not change the approved input.
- Backend inventory is authoritative; do not enqueue from arbitrary S3 frame-created events.
- The seal records saved/dropped/failed counts and real completion of background writers.
- If capture is interrupted, recover and review the inventory before sealing.
- Bounded upload windows avoid minting every URL long before it will be used.

## 5. Stitcher job decomposition

The unit of work is a sealed capture run or explicitly qualified segment, not automatically one burst folder per label.

```text
 +------------------------------ JOB INPUT CONTRACT ----------------------------+
 | run ID + attempt ID + sealed input digest                                    |
 | exact raw object references + approved asset bundle                          |
 | algorithm image digest + profile + direction policy + resource limits        |
 +-----------------------------------|-----------------------------------------+
                                     v
                         +-------------------------+
                         | 1. Stage and verify     |
                         | raw bytes + assets      |
                         +------------|------------+
                                      v
                         +-------------------------+
                         | 2. Normalize input      |
                         | flat chronological list |
                         | preserve original map   |
                         +------------|------------+
                                      v
                         +-------------------------+
                         | 3. Detect direction     |
                         | match anchor/content    |
                         +------------|------------+
                                      v
                         +-------------------------+
                         | 4. Find boundaries      |
                         | extract complete items  |
                         +------------|------------+
                                      v
                    +-----------------|------------------+
                    | Feature spans multiple frames?     |
                    +--------------|---------------------+
                              YES  |             NO
                                   v              |
                    +-------------------------+   |
                    | 5. Register frame pairs |   |
                    | SIFT + RANSAC affine    |   |
                    | retry finer scales      |   |
                    | hard-cut composite      |   |
                    +--------------|----------+   |
                                   +--------------+
                                          |
                                          v
                         +------------------------------+
                         | 6. Decode QR                 |
                         | classical -> erosion -> CNN  |
                         +--------------|---------------+
                                        v
                         +------------------------------+
                         | 7. Regroup physical labels   |
                         | anchor + layout association  |
                         | serial lookup + range check  |
                         +--------------|---------------+
                                        v
                         +------------------------------+
                         | 8. Render configured crops   |
                         | named DUST slots             |
                         | supplied profiles: 640x480   |
                         +--------------|---------------+
                                        v
 +---------------------------- ALGORITHM OUTPUTS -------------------------------+
 | crops/reel.json       keyed identifier manifest, schema v2                   |
 | labels.json           stitcher summary, schema v1                            |
 | labels.csv            order, identities, source/feature diagnostics          |
 | crops/*.png           actual enrollment images, in per-label directories     |
 | frames.csv            processing frame index -> original source file         |
 | warnings/performance  capture gaps, stitch/QC diagnostics and phase timing    |
 | checkpoints/logs      recovery and diagnosis                                 |
 +-----------------------------------|-----------------------------------------+
                                     v
                      +----------------------------------+
                      | 9. Result importer / validator   |
                      | decode files; verify inventory   |
                      | required slots; duplicate paths  |
                      | identity and position findings   |
                      +-----------------|----------------+
                                        v
                      +----------------------------------+
                      | Publish immutable attempt        |
                      | artifact hashes + completion     |
                      | execution != quality status      |
                      +-----------------|----------------+
                                        v
                                  DESKTOP REVIEW
```

Important boundaries:

- `stream_*` is already flat; nested `burst_*` needs an explicit, tested adapter.
- No stitching across unknown capture gaps or separate bursts merely because filenames sort.
- Do not replace the algorithm's hard-cut composition with generic panorama blending.
- Current long-label config extracts three shields but exports one DUST slot. The approved `reel.identifiers` definition decides what is enrolled.
- Preserve source-reported quality findings; do not invent an overall confidence probability.
- A correct-looking image and a decoded QR do not prove that they belong to the same physical label. Qualification must test that association.

## 6. Review and immutable approval boundary

```text
                 PUBLISHED PROCESSING ATTEMPT
                             |
                             v
                +------------------------------+
                | Candidate list + findings    |
                | label index / anchor         |
                | QR / serial + provenance     |
                | selected crop slots + hashes |
                +--------------|---------------+
                               |
                               v
 +--------------------------- OPERATOR REVIEW ----------------------------------+
 |                                                                             |
 | Inspect full crop <-> source neighbors <-> QR/serial                          |
 | Confirm feed direction, first/last labels and representative samples          |
 | Resolve capture gaps, missing slots and identity/association conflicts        |
 | Map scan order / reference ordinal -> explicit destination Reel positions     |
 |                                                                             |
 +----------------------------|------------------------------------------------+
                              v
                 +----------------------------+
                 | All required checks pass?  |
                 +-----------|----------------+
                        NO   |          YES
                         |   |           |
                         v   |           v
                 +----------------+  +-----------------------------------------+
                 | Reprocess or   |  | Freeze enrollment plan                  |
                 | recapture      |  |                                         |
                 |                |  | output revision + selected image hashes |
                 | NEW attempt    |  | identities + slot metadata + positions  |
                 +----------------+  | org/Team/Reel UUID + indexing            |
                                     | reviewer identity + approval digest     |
                                     +--------------------|--------------------+
                                                          v
                                            +---------------------------+
                                            | Real one-label pilot      |
                                            | at its FINAL position     |
                                            +-------------|-------------+
                                                          v
                                            +---------------------------+
                                            | Verify APID response      |
                                            | then enroll remainder     |
                                            +---------------------------+
```

Any change to an approved crop, identity, mapping, destination or indexing invalidates approval. A reprocessed output is a new attempt, never a replacement underneath a running enrollment plan.

Default v1 behavior: unresolved required labels block approval. If partial enrollment is later allowed, retain explicit gaps. Never compact positions after skipping a label.

## 7. Direct APID enrollment sequence

The keyed manifest includes per-identifier names/descriptions, so implement the general extraction-plus-members path. The single-image shortcut is optional, not the only supported path.

```text
 WORKFLOW DB          ENROLL WORKER            S3             AUTHD          APID
     |                      |                  |                |              |
     |<-- claim plan/row ---|                  |                |              |
     |--- frozen intent --->|                  |                |              |
     |                      |                  |                |              |
     |                      |-- API key token exchange -------->|              |
     |                      |<-- short-lived JWT ---------------|              |
     |                      |                  |                               |
     |                      |---------------- get selected Reel -------------->|
     |                      |<--------------- validate context/status ---------|
     |                      |                  |                               |
     |                      |-- GET crop ----->|                               |
     |                      |<== crop bytes ===|                               |
     |                      |                  |                               |
     |<-- extraction intent |                  |                               |
     |                      |---------------- POST /api/v1/tags/extract ------->|
     |                      |                 multipart: data=<crop bytes>     |
     |                      |<--------------- fingerprint id ------------------|
     |<-- save slot result -|                  |                               |
     |                      |                  |                               |
     |                      |         repeat for each approved DUST slot       |
     |                      |                  |                               |
     |<-- create intent ----|                  |                               |
     |                      |---------------- POST Reel Label ---------------->|
     |                      |                 explicit position                |
     |                      |                 all DUST fingerprints            |
     |                      |                 TEXT + QR + slot metadata        |
     |                      |                 options.indexing                 |
     |                      |<--------------- outcome + Label/member IDs ------|
     |                      |                  |                               |
     |                      |-- verify Reel/position/values/counts/indexing     |
     |<-- persist receipt --|                  |                               |
     |                      |                  |                               |
     |                      |---------------- GET Reel detail ---------------->|
     |                      |<--------------- ordered Labels + warnings -------|
     |<-- reconciled report |                  |                               |
```

The S3 exchange above represents an ordinary GET request followed by response bytes; APID receives bytes from the worker, not a reference to the bucket.

### Existing HTTP contract

```text
AUTHD
  GET <authd-base>/api/auth/token
  x-api-key: <service-account-key>

APID HEADERS
  Authorization: Bearer <returned-token>
  Dust-Ctx-Org-Id: <org-uuid>
  Dust-Ctx-Team-Id: <team-uuid>

COLLECTION / REEL SETUP
  GET  /api/v1/composite-tags/collections
  POST /api/v1/composite-tags/collections
       JSON: { name, description? }

  GET  /api/v1/composite-tags/reels
  POST /api/v1/composite-tags/reels
       JSON: { name, collectionId?, expectedIdentifiers? }

EXTRACT ONE CROP
  POST /api/v1/tags/extract
       multipart: data=<image bytes>
       response: fingerprint id

CREATE ONE LABEL WITH ALL ITS MEMBERS
  POST /api/v1/composite-tags/reels/{reelCollectionId}/labels
       multipart:
         position="<positive integer>"
         identifiers=<JSON-encoded member array>
         options={"indexing":"none"}      # JSON-encoded form field

       members:
         DUST: fingerprintId, name?, description?
         TEXT: value, name?, description?
         QR:   value, name?, description?

       outcome: created | already_enrolled

RECONCILE
  GET /api/v1/composite-tags/reels/{reelCollectionId}
```

```text
UI policy                 APID wire value
---------------------     ----------------
Verify-only               none
Identifiable              default
```

For one unnamed crop, the Label-create request may instead contain `data`, `humanReadable`, `qrValue`, `position` and `options` directly. APID extracts/enrolls that image internally.

Reel names are not unique. Persist returned UUIDs. Start the supervised pilot with an existing Reel if necessary; do not blindly retry an uncertain Reel creation based on its name.

## 8. Durable state model

### Entity relationships

```text
 AUTHORIZED PROJECT
       |
       +-- destination environment + org + Team
       +-- approved label profiles / reference-data versions
       |
       +----< CAPTURE RUN
                   |
                   +-- immutable raw inventory + capture-loss counters
                   |
                   +----< PROCESSING ATTEMPT
                               |
                               +-- pinned input/assets/image digests
                               +-- cloud Job ID + checkpoints + execution state
                               |
                               +----< LABEL CANDIDATE
                               |           |
                               |           +----< CROP ARTIFACT / NAMED SLOT
                               |           +----< FINDING / REVIEW DECISION
                               |
                               +----< ENROLLMENT PLAN
                                           |
                                           +-- immutable approval digest
                                           +-- target Reel UUID + position map
                                           |
                                           +----< ENROLLMENT ROW
                                                       |
                                                       +----< EXTRACTION RECEIPT
                                                       +----< API ATTEMPT / ERROR
                                                       +----> APID LABEL + MEMBERS
```

For v1, one plan selects one processing attempt and one target Reel. Supporting rescanned segments later requires an explicit merge/position policy, not a changed interpretation of existing IDs.

### State transitions

```text
CAPTURE / UPLOAD
  capturing -> flushing -> sealed -> uploading -> raw_verified
      |            |                     |
      +------------+---------------------+--> needs_attention / retry

PROCESSING
  queued -> staging -> matching -> extracting -> rendering -> publishing
                                                                   |
                  +------------------------------------------------+
                  |
                  v
             awaiting_review          execution finished; QC still applies
                  |
                  +--> accepted output / needs_attention

  Any active phase -> failed / cancellation_requested -> cancelled
  Retry -> new attempt or qualified exact-version checkpoint restore

ENROLLMENT PLAN
  draft -> approved -> pilot_running -> pilot_verified -> enrolling
                                                            |
                                                            v
                                                        reconciling
                                                            |
                                               +------------+------------+
                                               |                         |
                                               v                         v
                                           completed              needs_attention

  enrolling -> pause_requested -> paused -> resume same immutable plan
  cancel stops new work; it does not roll back completed APID writes

ENROLLMENT ROW
  pending -> extracting -> extracted -> submitting -> verified
                  |                         |
                  v                         v
             retry / review             reconciling
                                        /        \
                                  compatible    conflict / uncertain
                                      |                |
                                      v                v
                                   verified       operator action
```

### Source-of-truth boundaries

```text
 Station filesystem + SQLite    S3                 Workflow PostgreSQL       APID
 ----------------------------   ----------------   -----------------------   -----------------
 unuploaded raw bytes            sealed raw bytes   jobs + leases + outbox    actual Reels
 capture manifest                output versions   reviews + approvals       actual Labels
 durable upload progress         crop artifacts    frozen enrollment intent  actual membership
 rebuildable cloud cache         checkpoints       receipts + audit          indexing/state

 Local capture authority -----> immutable archive
 Desktop cloud status <--------- rebuildable from backend
 Workflow completion <---------- reconciled against APID, not merely local success counts
```

## 9. Failure, retry and reconciliation design

```text
                      +--------------------------------------+
                      | Persist intent before remote mutation |
                      +-------------------|------------------+
                                          v
                                  SEND SAME REQUEST
                                          |
                    +---------------------+----------------------+
                    |                     |                      |
                    v                     v                      v
             Valid success         Explicit failure       No reliable answer
                    |                     |               timeout / lost reply
                    v                     v                      |
             Validate receipt       Classify error              v
                    |                /          \       Mark RECONCILING
                    v          transient        permanent        |
              Checkpoint           |                 |           v
                    |          bounded retry     needs review   Read APID state
                    v              |                             |
                 VERIFIED          +---- same intent             v
                                                    +-------------------------+
                                                    | Proves compatible result?|
                                                    +-----------|-------------+
                                                           YES | NO/uncertain
                                                            |  |      |
                                                            v  |      v
                                                      checkpoint   conflict/review
```

Rules:

- One active owner per destination Reel, plus row leases and fencing in the workflow store.
- An expired lease does not prove that an old HTTP request failed to commit.
- APID's DUST-based retry reconciliation is conditional: same Team, Reel, position and compatible Label state.
- Any 409 is not automatically success; returned IDs, values and state must match intent.
- External extraction/enrollment is not one global database transaction. Do not promise universal exactly-once execution.
- An uncertain Collection/Reel creation needs manual reconciliation or a future server-supported idempotency contract.
- 401: one synchronized token renewal, then retry/reconcile or pause.
- 403: permission action; 429: respect rate limiting; invalid content: review, not an infinite retry.
- Pause/cancel never claims to undo already-enrolled Labels.

### Repository issues to fix before unattended use

```text
CURRENT GAP                                 REQUIRED BOUNDARY
-----------------------------------------   ---------------------------------------
Wrapper hides failed stitcher exit code ---> accurate child exit + regression test
Missing crops can accompany exit 0 -------> output/QC validation, separate status
Size-only upload skipping ----------------> checksum/version inventory verification
No sealed whole-run upload contract ------> flush barrier + verified upload commit
Frame-name/composite-only resume checks --> input/image/profile-bound checkpoints
Serial-derived crop path collisions ------> unique paths + pre-render collision gate
Mutable resume clears previous crops -----> new scratch attempt + immutable publish
```

The exit-code issue was reproduced using an isolated stub: child exit 42 became wrapper exit 0. Other listed risks were established by code review; live end-to-end operation remains a phase-0 validation task.

## 10. Security and identity boundaries

```text
                        OPERATOR IDENTITY
                              |
                              v
                     Google browser + PKCE
                              |
                  Tauri native token manager
                     (OS credential storage)
                              |
                         Google ID token
                              |
                              v
                       Workflow API
                   validates audience/issuer
                   maps user -> project rights
                              |
               +--------------+----------------------+
               |                                     |
               v                                     v
       scoped upload/read URLs               approved enrollment command
               |                                     |
               v                                     v
              S3                             Enrollment worker
                                                     |
                                              team-scoped secret
                                              from secret manager
                                                     |
                                                     v
                                                   AuthD
                                                     |
                                              short-lived APID JWT
                                                     |
                                                     v
                                                   APID
                                            org + Team authorization

       Stitcher Job -> workload IAM -> approved S3 inputs/outputs only
       Stitcher Job -X-> no APID service-account secret
       Desktop      -X-> no Kubernetes credentials / AWS master keys
```

Google login and APID authentication are not interchangeable. Project authorization must constrain the backend's privileged actions; a request cannot choose an arbitrary Team, bucket, source URI or executable configuration.

Record the approving operator separately from the executing service principal. Rust owns operator refresh credentials in Windows Credential Manager and validates the browser/PKCE callback; the renderer and Python helper do not receive tokens. Do not log keys, bearer tokens or signed URL query strings.

Tauri uses bundled local assets, CSP and explicit custom-command/window permissions; remote reports/pages cannot acquire native capabilities. Rust still validates paths, command state and allowlisted API/artifact origins. Capability configuration is not a sandbox around native Rust. No generic shell/spawn or unrestricted filesystem access is exposed to the web UI.

## 11. S3 layout and version boundaries

Proposed namespace; an adapter must support the current upload service's legacy `<folder>/<fileName>` layout.

```text
s3://<private-bucket>/projects/<project-id>/
|
+-- captures/
|   +-- <run-id>/
|       +-- raw/<original-relative-files>
|       +-- capture-manifest.json
|       +-- raw-complete.json
|
+-- assets/
|   +-- <asset-bundle-digest>/
|       +-- configs/
|       +-- templates/
|       +-- masks/
|       +-- approved-reference-data/
|
+-- processing/
|   +-- <run-id>/
|       +-- <attempt-id>/
|           +-- algorithm/
|           |   +-- crops/reel.json
|           |   +-- crops/<label-directory>/<slot>.png
|           |   +-- labels.json / labels.csv / frames.csv
|           |   +-- warnings.txt / performance.json
|           |   +-- stitched/ / checkpoints/ / logs/
|           +-- result-manifest.json
|           +-- thumbnails/
|
+-- enrollment/
    +-- <plan-id>/
        +-- approved-plan.json
        +-- report.json
        +-- report.csv
```

- Old capture folders are registered with their actual bucket/prefix and imported inventory.
- Sealed input is pinned by object versions/digests or made immutable; broad sync of a mutable prefix is not sufficient.
- Algorithm image and baked-in model versions are pinned by image digest; external assets use a bundle digest.
- New processing attempts publish to new output prefixes.
- Enrollment plan digest binds exact output artifacts and target context.
- Checkpoints require retained scratch storage or explicit verified restoration. Fresh ephemeral Jobs do not resume automatically.
- Raw evidence, approved crops/receipts, optional giant reports and scratch data have separate retention policies.

## 12. If enrollment must happen directly from Windows

This is an alternative deployment, not a second v1 implementation requirement.

```text
                         RECOMMENDED                      DESKTOP-DIRECT OPTION
                         -----------------------------    ----------------------------
Capture                  Desktop                          Desktop
Raw archive              S3                               S3
Stitching                Cloud Rust Job                   Cloud Rust Job
Review/approval          Desktop + workflow backend       Desktop + workflow backend
APID calls               Cloud enrollment worker          Native Rust desktop worker
Enrollment journal       PostgreSQL                       Local journal + cloud receipts
APID secret/token        Cloud secret manager/memory      OS credential store/memory
Cross-station ownership  Backend Reel lease               Backend Reel lease
After desktop closes     Approved cloud work continues    Enrollment pauses/stops
```

```text
 S3 crops ===> Desktop enrollment worker ===> APID
                    |
                    +--> local durable row journal
                    +--> backend destination-Reel lease and receipts
```

The HTTP fields, immutable approval, explicit positions and reconciliation rules remain identical. This removes the cloud enrollment-worker role but introduces workstation availability and credential-management requirements. It does not make APID enrollment available offline.

## 13. Implementation breakdown

Delivery order for **each capability**, distinct from runtime data flow:

```text
 BACKEND             API              CLIENT              SCRIPTS             UI
 domain/state ----> HTTP or IPC ----> nonvisual --------> end-to-end -------> screens
 unit tests         contract tests   frontend adapter    assertions          smoke tests
                                         |                  |
                                         +-- same code -----+
                                              |
                                              +--------------------------> UI binds it

 SCRIPT ENVIRONMENT
 test-only runner --> production client --> real new Workflow process + test DB
                                       \-> actual helper/native command core + journal
 external S3/runner/APID/auth fixtures: declared fakes in CI, separate live opt-in
```

No window, direct runtime DB writes, production approval shortcut or separate mock backend should be required to prove a capability. Scripts record exact IDs/digests/state, failures and restart behavior, then the UI consumes the same tested client. Unit/API tests run earlier too. Passing synthetic tests is not live cloud/scientific qualification.

Milestone outcomes (the dependency tables, not this overview, govern ordering):

```text
 M0: PROVE THE CONTRACTS
     Real approved S3 run -> pinned stitcher -> verified crop/QR/serial
                         -> direct APID pilot -> returned member validation
       |
       v
 M1: DURABLE INGEST
     Capture seal + upload journal + checksums + registry + project access
       |
       v
 M2: CLOUD PROCESSING
     Job scheduler + status recovery + pinned staging + result importer
       |
       v
 M3: REVIEW / APPROVAL
     Review/approval API + client + scripts -> candidate/approval UI
       |
       v
 M4: DIRECT ENROLLMENT
     APID worker + row/recovery/report APIs + scripts -> enrollment/results UI
       |
       v
 M5: SHIP / OPERATE
     Signed Windows installer + monitoring + retention + full-Reel field pilot
```

The [implementation backlog](IMPLEMENTATION_SLICES.md) retains 25 parent IDs and separates backend/script readiness from UI completion. After S01, prioritize S05-S09's service/API proofs while engine/headless-helper work follows its own prerequisites. S10a is now nonvisual frontend integration after S09; S10b adds the Tauri shell only after scripts pass. Review/approval/enrollment/report backend gates (S11a/S12a/S13a/S15a/S16a) do not depend on their screens. S20a scripts the full headless journey before S20b capture UI; S21 qualifies the release bundle. This supersedes the earlier shell-first sequencing. No automatic real approval/enrollment is introduced.

## 14. Decisions that change this diagram

1. Is cloud processing infrastructure already operated, and which environment should own it?
2. Is the desktop the capture station, a separate review station, or both?
3. Must enrollment HTTP originate on Windows, or may a cloud worker act for the operator?
4. Which DUST slots are required for each label profile?
5. What defines the printed serial and physical Reel position, including reverse feed/rescans?
6. Is enrollment verify-only or identifiable by default for this project?
7. Are partial Reels allowed, or must every required label be resolved before approval?

Selected desktop direction: Tauri + bundled web UI with a retained Python capture helper. Proposed defaults still awaiting the relevant S00 approvals: React/TypeScript/Vite, cloud processing/enrollment, one qualified profile, explicit operator approval and no silent gaps. No clid dependency or local Windows stitcher is introduced.
