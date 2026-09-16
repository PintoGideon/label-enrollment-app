# ORCH-01 — Processing orchestration through result approval

**Status:** user confirmed one delivery slice and **local development with the
existing repository stitcher first**. No cloud stitcher exists. Local launch
qualification and processing control checkpoints have passed locally. Result
review/approval passed the actual-service/client synthetic proof. Development is
now restricted to a **50-frame contiguous subset** of the provided stream; larger
runs are deferred by user instruction. The latest fetched engine produced 12
complete crops on that subset; missing serial authority still blocks approval.
Do not claim full slice or scientific acceptance
from engine exit, fixture approval or a simulated runner alone.
**Base:** `31c589f` — authenticated project listing, following foundation `23df567`.
**Commit/push branch:** `slice/s01a-project-list`, consolidated onto the previous
published slice at the user's request. The local `slice/orch01-processing-orchestration`
branch was only an uncommitted working branch; no history rewrite is required.

## Outcome

Through the actual Workflow API and reusable client, an authorized operator can
register existing S3 input, verify it, run the real stitcher, observe durable
progress, inspect validated results and explicitly approve a frozen result.
The signed-in human remains accountable throughout.

```text
Operator / nonvisual client
          |
          v
Local Go Workflow + PostgreSQL + S3-compatible development storage
  authorize -> register run -> verify immutable object input
          |
          v
Existing Rust stitcher in a local runtime
  real processing -> progress + attempt-specific outputs
          |
          v
Workflow validates outputs -> review -> immutable result approval
                                              |
                                         STOP FOR THIS SLICE
```

**Development now:** run Workflow and the real stitcher locally, driven by the
external harness. Use controlled local S3-compatible storage for object API,
inventory/version and checksum cases; do not replace verified object input with
an unqualified filesystem shortcut. No live AWS access is implied.

**Deployment later:** cloud Workflow and cloud stitching remain the product target,
but neither a pre-existing cloud stitcher nor a hosting decision blocks this slice.
There is no Kubernetes requirement. Local success is not cloud-operation evidence.

## One slice, six included capabilities

| Capability | Required behavior |
|---|---|
| Command permissions | Check verified identity, project/object scope and action permission for every read/mutation. Recognize verified human versus service identity where ownership/approval requires a person. Server-owned storage/profile/runner policy constrains privileged execution; submitted actor/context headers never grant authority. |
| Durable runs | Persist human owner, individual action actors, input revisions, attempts, commands and events using pgx/sqlc/Goose. Identical command replay returns the original outcome; changed-payload reuse and stale revisions fail. State survives orchestrator restart. |
| Verified S3 inputs | Start with the approved existing-S3, flat-stream path. Allowlist sources, paginate the complete inventory, verify bytes/checksums and pin versions or an immutable snapshot. Record the manifest digest and real capture metadata; unknown history stays unknown. No processing before verification. |
| Stitcher control/progress | A narrow adapter starts, observes and cancels the existing Rust engine. Persist execution identity, leases/fencing and actual stage/count events. Reconcile uncertain submission before retrying. Do not expose arbitrary commands, profiles or cloud credentials to clients. |
| Result validation | Import the actual engine output contract, including `crops/reel.json`; verify selected slots, identities, paths, image bytes/dimensions, counts and provenance. Publish immutable artifact references and authorized inspection APIs. Process exit alone is not successful validation. |
| Approval | Expose explicit review/approval commands. Record the verified human approver and freeze the exact input/result/profile/reference-data revisions and required review decisions. Reprocessing or changing approved content invalidates eligibility; old approval records remain auditable. |

**Approval boundary for this implementation:** approval of the reviewed processing result, not
permission to write to APID. Destination-specific enrollment planning (Team/Reel,
positions and indexing) needs its own frozen approval in the later APID slice.
Do not reuse a processing approval as blanket enrollment authorization.

## First-cut executable contract

- Current project membership is local processing authority: `capture` registers;
  `process` verifies, starts/reprocesses and cancels; `review` reviews/approves.
  Any member may inspect. All commands require a verified human. Ownership does
  not bypass roles; worker dispatch/continuation rechecks the initiating role.
  No APID/Team authority is inferred from these memberships.
- Administrator policy selects local storage endpoint, source/project prefixes,
  immutable image identity and explicit profile assets. The local-only policy
  does not enable live S3, cloud execution or unqualified production profiles.
- Registration declares an exact sorted file/checksum inventory. Approval needs
  an expected serial sequence; an explicit inspection profile can convert a
  provided stream without inventing that sequence or serial authority, and must
  remain approval-blocked. Capture history remains unknown. Processing uses a sealed
  byte snapshot, not mutable source keys or unsafe algorithm resume.
- Commands carry UUID idempotency keys and expected run revision. Reviews bind
  every expected label and warning acknowledgement to one result digest;
  approval freezes that review plus input/profile/reference/image provenance.
- Local launch passed using the pinned Rust engine, OpenCV 4.14.0 and an
  external synthetic fixture: 2 frames, 1 real registered composite, 1 complete
  QR/serial-associated label and a 640×480 crop, no warnings. Build/help and
  nonzero invalid-input exit also passed. This is launch/fixture evidence only,
  not the integrated Workflow acceptance gate. Detailed reports stay external.

## Execution and recovery boundaries

- Define one deployment-independent processing contract and implement a local
  execution adapter for the real repository stitcher. Prefer the repository's
  local Docker build path on this workstation to isolate its OpenCV 4 dependencies.
  Qualify launch first, then wire Workflow to it. Cloud adapters come later;
  do not implement multiple hosting platforms or a generic workflow engine.
- The stitcher remains an image processor, not an AuthD login server, Workflow
  coordinator or APID enroller. No user token or APID secret in its arguments,
  environment, artifacts or work payloads.
- Separate execution, validation and review states. Missing/corrupt required
  outputs block approval even when the executable reports success.
- Reconnect to a still-running attempt after orchestrator restart. If execution
  ownership or completion is uncertain, reconcile or expose that uncertainty;
  do not blindly launch a duplicate or infer success from an output prefix.
- Cancellation requires a confirmed terminal outcome or an explicit uncertain
  state. Fence stale progress/result updates and give each attempt separate output.
- Start with verified fresh executions. Algorithm checkpoint reuse is allowed
  only after its separate safety prerequisites pass; otherwise create a new
  attempt from the verified input. Never resume into approved published output.
- Invoke `stitchin-complete` directly rather than `pipeline`/`run_logged`, whose
  reviewed wrapper can swallow failures. Preserve the executable's real exit
  status and still validate outputs independently. Do not change the reference
  checkout to conceal the wrapper defect.

## Local launch contract and first proof

Reviewed source: `dustid/labeltron-two-stitcher` at
`d78c82d9cc6ab0ac54f9dfe9a997a8fe6547effa`, in the external reference checkout.
Reuse that implementation; do not vendor it into this app or use the study
checkout as runtime asset/output storage.

The actual CLI in `src/bin/stitchin-complete.rs` supports:

```text
stitchin-complete <approved-config.yaml>
  --source <verified-staged-frames>
  --name <server-generated-attempt-id>
  --out-root <owned-output-root>
  --jobs <bounded-worker-count>
  --direction <approved-direction>
```

This is the source-verified invocation shape, not evidence of a successful run.
Use an isolated working directory for config-relative templates/masks/models and
synthetic mapping data. Input/assets are read-only; outputs/logs are owned per
attempt. No `--overwrite` or `--resume` in the first fresh-run adapter.

First build/launch qualification, entirely through the external harness:

1. Build the pinned engine with its lockfile and OpenCV 4 dependencies. Local
   Docker is available; the native OpenCV 4/LLVM/CMake setup is not yet installed.
   Stage only explicitly permitted source/build files into an owned external
   build context. Never send/mount the protected `manifests/` or use the repo's
   default Compose mounts; no reference checkout modifications.
2. Invoke the compiled binary's `--help`, then a controlled invalid-input case
   that must return nonzero. In a container, select the actual executable as the
   entrypoint, not the defective logging wrapper. Record the artifact identity.
3. Run approved synthetic frames/profile/assets through the actual pipeline;
   capture outputs, exit status and available progress evidence. The profile
   and expected outputs must be explicit before accepting this proof.
4. Connect this same real execution path to Workflow start/status/cancel/recovery.
   Do not present `--help` or a successful image build as workflow acceptance.

`performance.json`, console stage output, `run.yaml` and result inventories are
source-visible integration surfaces. Qualify their update/failure semantics;
console text alone is not durable progress or successful output validation.

## Implementation steps within this slice

- [x] Qualify the local build/launch of the existing real stitcher, then finalize
  processing/approval contracts, permission matrix and synthetic profile/input
  fixture. Keep live operations fail-closed until their authority/resource policy
  is qualified.
- [x] Add the durable schema, transactional command/event handling and authorized
  run APIs. Extend the actual Go client and external harness as these land.
- [x] Add S3 inventory verification and immutable input registration, including
  recovery across storage/database boundaries.
- [x] Add the local processing adapter, durable dispatch/status/progress/cancellation
  and attempt recovery; connect the qualified repository stitcher without folding
  its algorithm into Workflow. Do not wait for or implement cloud deployment.
- [x] Add immutable result import/validation, authorized inspection, review and
  result-approval APIs and matching nonvisual client methods.
- [ ] Run the complete external proof with the real service/client/PostgreSQL and
  actual stitcher; exercise denial, corrupt inputs/results, duplicate commands,
  failed execution, cancellation, restart and stale-approval cases. Record what
  was local versus cloud and which external dependencies were controlled fixtures.

These are implementation checkpoints, not separately delivered product slices.
The slice's completion gate is the integrated workflow through approval.

## Acceptance

```text
List/select authorized project
 -> register run
 -> verify fixed S3 input inventory
 -> start real stitcher
 -> observe durable stage/count progress
 -> validate and inspect exact results
 -> submit authorized review/approval
 -> retrieve the same frozen approval after service restart
```

Also demonstrate:

- Unauthorized/cross-project operations and spoofed owner/approver identities fail.
- Repeated commands do not duplicate runs, attempts or approvals.
- Source mutation, missing inputs and invalid result artifacts cannot advance.
- Failed/terminated execution never becomes successful merely because files exist.
- Restart/cancellation/ambiguous submission preserve evidence and do not cause
  blind duplicate execution; stale attempts cannot publish into a newer attempt.
- Approval is explicit, version-bound and unavailable for incomplete results.
- Zero APID calls, including extraction, occur in this slice.

All new proof scripts, fixtures and reports live in `../label-enrollment-harness/`.
Use the actual service and production client, not copied backend logic. Owned
PostgreSQL only; direct SQL is limited to fixtures/fault injection/assertions.
No new application test files for this first cut; existing foundation tests stay.
A controlled S3-compatible store/runner is useful for fault cases, but clearly
label it. Real-stitcher integration and live-cloud qualification are distinct
from simulated-runner checks. Synthetic approval never approves real label data.

## Local integration decisions and remaining prerequisites

1. **Runner locally proven:** existing Rust/OpenCV stitcher in local Docker;
   no cloud stitcher is available or required. The synthetic fixture proved an
   actual registered composite, with real API-controlled execution and recovery.
2. **50-frame development baseline:** original sorted frame indices 2400–2449
   from `test dataset/stream_20260908_120631`, 40,842,163 bytes. Harness enforces
   exactly 50 frames for provided-stream tests; defer larger runs. A separate
   sparse, harness-owned checkout fetched upstream `main` at
   `7e67d9252faae2bb694ffa9ad297031c4066ebf0`; reference checkouts and protected
   manifests remain untouched. Latest-source processing produced 12 complete
   640×480 crops and 12 distinct decoded QR values in 25.60 seconds, with one
   skipped, degenerate stitch at the subset's first pair. All original output
   files were archived/versioned/read-back verified in local MinIO. Missing serial
   authority deliberately blocks approval. See external `reports/stream-50-latest.md`.
   Synthetic result/review/approval replay, restart and reprocessing checks also
   passed, but the remaining full fault/recovery acceptance gate is not complete.
3. **Storage:** use controlled S3-compatible development storage for the local
   workflow proof. Live S3/IAM and AuthD/current org/Team authority are later
   qualification gates; no worker credential substitutes for operator permission.
   Fixtures use the real JWT verifier and APIs, not a shipped authentication bypass.

## Backlog relationship and exclusions

This is a grouped delivery slice drawing on command authorization from S05,
S06-S09, and the review/result-approval portion of S11a. The existing IDs remain
traceability/checklist references, not separate user-facing delivery goals.
Engine safety prerequisites remain applicable. This does not automatically pass
full S00/S01/S05, scientific, cloud-operation or release gates.

Out of scope: APID extraction/enrollment/reconciliation, APID destination-specific
approval (full S12a), capture/upload UI, new camera behavior, desktop packaging,
cloud provisioning/platform selection, generic burst flattening, algorithm
redesign and unsafe checkpoint reuse. No clid dependency or reference vendoring.
