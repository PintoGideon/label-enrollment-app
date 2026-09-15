# Labeltron Enrollment - Implementation Slices and TODOs

**Status: proposed backlog. No implementation has started.** All checkboxes below are intentionally open. Repository-review findings are inputs to these tasks, not evidence that a task is finished.

Architecture: [SYSTEM_DESIGN.md](SYSTEM_DESIGN.md).
Detailed design: [DESKTOP_APP_PLAN.md](DESKTOP_APP_PLAN.md).
Source baseline: [SOURCES.md](SOURCES.md).

This document is the execution-order and TODO reference. The other documents explain the architecture; they are not competing task trackers.

## 1. How we will build

1. Prove the existing stitcher/APID contracts before investing in the complete UI.
2. Deliver a usable path for an **existing S3 run** first: import -> process -> review -> approve -> enroll -> report.
3. Build capture sealing and reliable uploads in parallel, then connect fresh captures to the same pipeline.
4. Keep each slice independently reviewable, with an observable demo and negative tests.
5. Keep production enrollment disabled until the release and field-approval gates pass.

A slice should normally fit in a few focused engineering days and one or a few small PRs. If refinement reveals more than about three days of implementation, split it into child tickets before coding. Native builds, infrastructure access and scientific qualification are uncertain; these are sizing guidelines, not delivery promises.

### Initial scope, subject to S00 approval

- Extend the existing Python/PyQt6 Windows app.
- Cloud Rust stitcher; cloud worker makes direct APID HTTP calls; no clid binary.
- One approved label profile and flat continuous streams first.
- One capture run / selected processing attempt per enrollment plan.
- Existing APID Collection/Reel selected by UUID for the first release.
- Explicit indexing and physical-position policy; never inherit API defaults silently.
- Required-label failures block plan approval. No silent fallback, skipped/re-numbered labels or automatic enrollment.
- Polling/phase progress first; no WebSocket/SSE infrastructure required.
- No algorithm rewrite, local Windows stitcher, new camera UI or multi-segment Reel merging.

## 2. Ownership and code boundaries

| Area | Implementation home | Notes |
|---|---|---|
| Desktop | `dustid/labeltron-two`, based on `jhodges/wininstaller` | Qt-free `pipeline/` logic, existing controller/worker bridge, small UI panels |
| Engine | `dustid/labeltron-two-stitcher` | Rust algorithm, wrapper, checkpoints and output contracts |
| Workflow | New backend; repository/location confirmed in S00 | One TypeScript codebase for HTTP API, scheduler, importer and enrollment worker |
| Contracts/tests | Versioned schemas and synthetic/golden fixtures | Same contract examples tested by Python, TypeScript and Rust where relevant |
| Storage/compute | Approved cloud infrastructure | One upload-signing authority; adapt the existing upload service or implement its successor route deliberately |
| APID | Existing HTTP API | No server change needed for supervised enrollment into an existing Reel |

Suggested Workflow module boundaries: `auth/`, `runs/`, `storage/`, `jobs/`, `results/`, `reviews/`, `enrollment/`, `apid/`. These are modules, not separate microservices. Introduce tables/routes as their slices need them; do not scaffold every future module in the first PR.

The `reference/` checkouts remain study material. Implementation will use explicit working branches in the chosen repositories. Do not initialize repositories, change those checkouts, install dependencies or provision infrastructure as part of approving this plan.

## 3. Slice index and dependencies

**S00 is in decision review**, with required decisions/access still pending; see its [decision sheet](plans/S00-pilot-scope.md). **S01-S24 are not started.** Dependencies mean the upstream acceptance gate has passed, not merely that its code exists. Only S00 is ready for review now.

| ID | Slice / observable outcome | Depends on | Area |
|---|---|---|---|
| S00 | Approved pilot scope, data and environment | None | Product / algorithm / platform |
| S01 | Executable contracts and safe fixtures | S00 | Shared |
| S02 | Fresh stitcher runs cannot report false success or overwrite colliding crops | S01 | Engine |
| S03 | Checkpoint reuse is content-bound and crash-safe | S02 | Engine |
| S04 | One known label enrolled through direct APID; adapter contract proven | S02 | Workflow / algorithm |
| S05 | Authenticated workflow service with project isolation | S01 | Workflow |
| S06 | Durable run registry, commands and events | S05 | Workflow |
| S07 | Existing S3 run imported as a verified immutable input | S06 | Workflow / storage |
| S08 | One cloud processing job with durable status and cancellation | S02, S03, S07 | Workflow / engine / platform |
| S09 | Validated, immutable result artifacts and candidate API | S08 | Workflow |
| S10 | Desktop can browse a cloud run and start/watch processing | S09 | Desktop |
| S11 | Operator reviews images, identities and exceptions | S10 | Desktop / Workflow |
| S12 | Destination/positions selected; exact enrollment plan approved | S04, S11 | Desktop / Workflow |
| S13 | One approved pilot label enrolled with a durable receipt | S12 | Desktop / Workflow |
| S14 | Lost API responses and interrupted rows reconcile safely | S13 | Workflow |
| S15 | Bounded bulk enrollment with pause/resume | S14 | Desktop / Workflow |
| S16 | Per-position reconciliation and downloadable final report | S15 | Desktop / Workflow |
| S17 | Capture produces atomic files and a sealed manifest | S01 | Desktop |
| S18 | Checksum-aware upload API and verified completion | S07 | Workflow / storage |
| S19 | Desktop uploads resume safely after restart | S10, S17, S18 | Desktop |
| S20 | Fresh capture enters the complete workflow without scripts | S16, S19 | Desktop / Workflow |
| S21 | Signed Windows installer and upgrade/rollback validation | S20 | Desktop / release |
| S22 | Production infrastructure, capacity, backups and operations ready | S16, S19 | Workflow / platform |
| S23 | Integrated failure/security rehearsal passes | S21, S22 | Cross-cutting |
| S24 | Controlled physical-Reel pilot and release decision | S23 | Operations / product / algorithm |

### Work lanes

```text
                               S00 -> S01
                                        |
                  +---------------------+------------------------+
                  |                     |                        |
                  v                     v                        v
                 S02                   S05                      S17
                /   \                   |
               v     v                  v
              S03   S04                S06
                                        |
                                        v
                                       S07 ---------> S18
                                        |
                         S02 + S03 -----+---> S08 -> S09 -> S10 -> S11
                                                                 |       |
                                              S17 + S18 ---------+       +---+
                                                                 v           |
                                                                S19          |
                                                                             v
                                                               S04 -------> S12
                                                                             |
                                                                             v
                                                                  S13 -> S14 -> S15
                                                                                 |
                                                                                 v
                                                                                S16
                                                                                 |
                                                 S19 + S16 ---------------------+
                                                          |
                                                +---------+---------+
                                                v                   v
                                               S20                 S22
                                                |
                                                v
                                               S21
                                                |
                                         S21 + S22 -> S23 -> S24
```

The table is authoritative if the overview diagram omits a dependency. S04 is an opt-in nonproduction technical probe; it does not unlock unattended/product enrollment. That only starts after review and durable intent exist in S12-S14.

## 4. Common definition of done

Every implementation slice must satisfy all of these, in addition to its own TODOs:

- [ ] Scope, owner, affected repo/branch and acceptance cases agreed before coding.
- [ ] Happy-path and relevant failure-path tests committed; real network/hardware tests opt-in.
- [ ] Schema changes have migrations/compatibility tests where applicable; no hand-edited generated APID artifacts.
- [ ] Authorization, bounded resources, secret redaction and safe path handling included where first introduced.
- [ ] State-changing operations have durable intent and defined retry/cancel behavior; no false exactly-once claims.
- [ ] Existing capture/simulator behavior still passes where affected; no UI-thread disk/network work.
- [ ] Feature can be disabled safely; rollback does not delete raw captures, approvals or receipts.
- [ ] Demo/evidence recorded, including what was mocked versus tested against real services.
- [ ] Human review accepted; update only the completed slice's checklist/status.

A later reliability/security slice is a system-level rehearsal, not permission to defer essential safeguards until release.

## 5. Detailed slice checklists

### S00 - Lock the pilot scope and unblock access

**Outcome:** one explicit decision sheet, not assumptions hidden in code.
**Depends on:** none. **Area:** product, algorithm owner and platform owner.
**Decision review:** [plans/S00-pilot-scope.md](plans/S00-pilot-scope.md), on `slice/s00-pilot-scope`. Required decisions and access remain pending.

TODO:
- [ ] Confirm integrated Qt desktop and cloud enrollment worker versus Windows-origin enrollment; record the decision.
- [ ] Confirm Workflow repository location, backend runtime, operated EKS versus Batch/ECS, nonproduction endpoints and owners.
- [ ] Choose the initial profile, required DUST slots, serial authority and reference-data version.
- [ ] Define scan-order/reference-order/APID-position mapping, including reverse feed and endpoint labels; define indexing and no-gap policy.
- [ ] Obtain an approved small raw run, representative edge cases and independently checked label-to-QR-to-shield ground truth. Use supplied copies outside protected `manifests/`; do not read/change that directory.
- [ ] Arrange a nonproduction Team-scoped service account, pre-created disposable Reel(s), approved S3 prefixes and explicit permission for the test writes.
- [ ] Record supported Windows/camera environment, target workload, storage retention and which checks require real hardware.

**Acceptance:** required scope/identity/position/indexing decisions are approved and prerequisite fixture/environment access is available. An unresolved required item keeps S00 blocked; merely naming its owner does not complete the slice. Normal development needs no production credentials/data.
**Not included:** implementation, dependency installation, real enrollment or infrastructure deployment.

### S01 - Define executable contracts and a safe test harness

**Outcome:** all components agree on input/output, identity and errors before integration.
**Depends on:** S00. **Area:** shared contracts and chosen Workflow test package.

TODO:
- [ ] Define minimal versioned `CaptureManifest`, `ProcessingAttempt`, `ResultManifest`, `LabelCandidate`, `EnrollmentPlan`, row receipt and event/error schemas.
- [ ] Distinguish run ID, attempt ID, candidate ID, plan ID, physical position, Reel UUID and command ID; specify enums and legal transitions.
- [ ] Specify canonical digest inputs and normalization: content/version identity, QR exactness, TEXT semantics, positions, target context and indexing; exclude signed URLs/tokens.
- [ ] Add synthetic/golden fixtures: valid and incomplete schema-v2 Reel documents, multiple named DUST slots, duplicate identity/path, unsafe path, reverse mapping and partial capture.
- [ ] Add fake HTTP/object-store/runner adapters plus deterministic clock and fault injection. Mocks must not be mistaken for cloud or scientific qualification.
- [ ] Establish focused CI for changed components; normal test runs require no camera, AWS, APID key or access to protected reference files.

**Acceptance:** Python/TypeScript consumers interpret golden identities/digests consistently; invalid fixtures fail with stable machine codes.
**Not included:** all future routes/tables, a new UI framework or a deployed service.

### S02 - Make fresh stitcher executions trustworthy

**Outcome:** a failed engine cannot look successful, and separate labels cannot silently overwrite each other's crop.
**Depends on:** S01. **Area:** Engine `docker/entrypoint.sh`, `src/labels.rs`, `src/bin/stitchin-complete.rs` and tests.

TODO:
- [ ] Fix `run_logged` to preserve the child exit status while retaining logs; define separate logging/upload/optional-report error handling.
- [ ] Add isolated wrapper tests for child exit 0, exit 42, missing executable, upload failure and interrupted execution.
- [ ] Preflight crop destination uniqueness before parallel rendering; for v1, reject duplicate/sanitization-colliding identities with structured diagnostics rather than rename paths silently. Preserve valid existing manifest/CSV-regeneration compatibility.
- [ ] Expose crop-render failures in a structured outcome; keep execution status separate from quality status and prevent optimistic CSV status from becoming authoritative.
- [ ] Treat zero labels, missing required files and partial JSON as explicit invalid/incomplete results; do not claim the shell exit alone validates a Reel.
- [ ] Verify a fresh run on an approved small fixture preserves crop pixels/QR association for valid inputs; pin the resulting test image by digest.

**Acceptance:** stub exit 42 remains nonzero, colliding crop jobs cannot overwrite, and a missing crop remains visible even if algorithm execution completes.
**Not included:** changing SIFT/QR thresholds, inventing confidence scores or enabling resume. Exit handling and crop protection may be two small PRs under this slice.

### S03 - Make checkpoint reuse safe

**Outcome:** restart reuses only work that is provably compatible with the same input.
**Depends on:** S02. **Area:** Engine `src/checkpoint.rs` and pipeline resume logic.

TODO:
- [ ] Bind checkpoint/composite identity to raw content digests, algorithm version, approved configuration and template/model/reference versions as applicable.
- [ ] Reject old/unknown cache versions and conservatively recompute when dependency provenance is missing; correctness before fine-grained cache optimization.
- [ ] Stop reusing a composite merely because a same-named decodable file exists.
- [ ] Repair a truncated final JSONL record to the last complete boundary before appending; retain corruption evidence and handle non-tail damage explicitly.
- [ ] Restrict resume to unpublished single-writer scratch workspaces. Published crops and approved attempts are never modified in place.
- [ ] Test same filenames/different bytes, changed image/profile, missing composite and repeated crash -> resume -> crash -> resume.

**Acceptance:** resumed output matches the corresponding clean run under the approved comparison policy; stale caches cannot produce an apparently valid result.
**Not included:** retained cloud volume provisioning or arbitrary cross-version cache reuse; S08 integrates storage recovery.

### S04 - Prove the direct APID adapter and technical vertical slice

**Outcome:** one approved known label reaches APID without clid, before the full product is built.
**Depends on:** S02 and its inherited prerequisites. **Area:** Workflow `apid/` adapter and opt-in integration harness.

TODO:
- [ ] Implement typed AuthD token exchange, org/Team context headers, bounded timeouts, redacted errors and synchronized token renewal.
- [ ] Implement Collection/Reel read methods, crop extraction, explicit-member Label creation and response validation; use an existing target Reel UUID only.
- [ ] Pin the approved profile/assets and run a fresh stitch of the S00 input; independently compare serial/QR/crop association, including first/last labels and direction cases supplied for this pilot.
- [ ] Send the exact selected crop bytes; test named single-DUST and multi-DUST multipart members plus `none`/`default` indexing mapping against fake contracts.
- [ ] With explicit opt-in, enroll one reviewed known label into the nonproduction Reel and save a durable, secret-free probe receipt with hashes and returned IDs; extraction is a write too.
- [ ] Verify live response shape, fingerprint reuse/expiry assumptions, permissions, scan-service availability, image limits and repeat/conflict behavior. Stop on ambiguous outcomes; do not build an automatic probe retry loop.
- [ ] Record real timing/memory/upload observations and contract discrepancies; turn blocking discrepancies into follow-up tickets before S12.

**Acceptance:** exact approved image -> extracted fingerprint -> Label/member IDs -> Reel detail agrees with the fixture; mocked versus live-tested cases are explicit.
**Not included:** production access, batch enrollment, auto-create Reels, UI or a claim of full profile qualification from one success.

### S05 - Stand up the authenticated Workflow boundary

**Outcome:** a minimal service knows who the operator is and which project they may access.
**Depends on:** S01. **Area:** Workflow `auth/`, service entry point and development deployment.

TODO:
- [ ] Add minimal service startup, health/readiness, configuration validation and PostgreSQL connection/migration harness.
- [ ] Validate Google token issuer, audience, signature and lifetime using maintained libraries; do not build a new authentication system.
- [ ] Map authenticated subjects to authorized projects and server-owned S3/APID context. Reject arbitrary client-supplied Team, bucket or image choices.
- [ ] Add `GET /pipeline/v1/projects`, basic roles for capture/process/review/enroll, request IDs and redacted structured logging.
- [ ] Test missing/expired/wrong-audience tokens, a disallowed project and attempted destination escalation; fail closed outside explicitly isolated tests.
- [ ] Define the backend API/worker roles in one codebase and establish a nonproduction smoke deployment or local isolated equivalent.

**Acceptance:** allowed caller sees only its configured projects; a forged subject/project/context cannot cause privileged work.
**Not included:** all pipeline features, AWS master keys in the desktop, Better Auth setup or new operator sign-up flows.

### S06 - Register runs and persist commands/events

**Outcome:** creating a run twice or restarting the service does not lose or duplicate its identity.
**Depends on:** S05. **Area:** Workflow `runs/`, PostgreSQL, outbox/events.

TODO:
- [ ] Add runs and minimal command/outbox/event tables with project ownership, input revision, origin, capture metadata and actual storage locator.
- [ ] Add idempotent `POST /pipeline/v1/runs`, authorized paginated list/detail, revision checks and stable command IDs.
- [ ] Return the original result for a repeated identical command; reject reuse of a command ID with changed semantics.
- [ ] Make state transition + durable event/outbox writes transactional; add safe event pagination/cursors for polling.
- [ ] Define intermediate/recovery states and timeouts instead of deriving completion from a local in-memory flag.
- [ ] Test concurrent duplicate submissions, restart, denied project access, stale revision and outbox replay.

**Acceptance:** repeated create returns one run ID; accepted commands/events survive process death; no cross-project leakage.
**Not included:** remote raw verification, a generic workflow framework or real job submission.

### S07 - Import an existing S3 run safely

**Outcome:** an already-uploaded run becomes a verified input for processing.
**Depends on:** S06. **Area:** Workflow `storage/` and run-import handler.

TODO:
- [ ] Accept a registered/allowlisted legacy prefix, not an arbitrary URI; paginate the complete object inventory and reject unsupported nested-burst input with an actionable message.
- [ ] Preserve source names/order and obtain reviewed capture metadata; unknown loss/timing history must stay unknown, not become zero.
- [ ] Pin each object version or create a verified immutable snapshot; compute/validate exact bytes and content digests, not sizes or ETag assumptions alone.
- [ ] Validate the whole inventory and ownership, then commit `raw_verified` with an input digest and outbox event; distinguish import errors from capture-quality findings.
- [ ] Specify crash recovery between S3 markers and PostgreSQL commits; neither resource is part of a shared transaction. Reconcile idempotently, and keep DB state as the scheduling authority.
- [ ] Test >1 page, changed same-size object, mutation during import, missing frame, duplicate completion and a forbidden prefix.

**Acceptance:** processing references a fixed verified frame set even if the old upload prefix later changes.
**Not included:** automatic processing on every S3 object-created event or generic recursive burst flattening.

### S08 - Run one cloud processing attempt

**Outcome:** an authorized run launches one bounded job, and its status survives service restart.
**Depends on:** S02, S03, S07. **Area:** Workflow `jobs/`, Engine staging wrapper and approved compute.

TODO:
- [ ] Add attempts/job leases and start/status/cancel routes; pin input digest, asset bundle, image digest and profile before enqueueing.
- [ ] Implement one runner adapter for the S00 runtime, including minimal nonproduction IAM, storage and central logging; do not implement EKS and Batch simultaneously.
- [ ] Stage only inventory-referenced versions/bytes; use validated SDK parameters and collision-free scratch paths, not user-controlled shell strings.
- [ ] Make job identity deterministic per attempt; recover a create-call timeout by discovering that job before submitting anything else. Persist runtime IDs and fence stale callbacks.
- [ ] Watch actual runtime exit/conditions/heartbeat and bounded phase progress; configure CPU/memory/disk/deadline/fleet limits.
- [ ] Implement cancellation and exact-version checkpoint restoration or retained-volume reuse. If checkpoints cannot be verified, rerun in new scratch; never pretend an empty volume is resumable.
- [ ] Test duplicate starts, lost submit response, worker restart, OOM, timeout, cancellation, failed result upload and recovery after final-upload code cannot run.

**Acceptance:** one attempt has one active runtime owner; a failed/killed job never becomes successful merely because an output prefix exists.
**Not included:** auto-enrollment, scientific quality approval or a separate scheduler microservice.

### S09 - Validate and publish processing results

**Outcome:** the backend offers trustworthy candidates/artifacts rather than raw console output.
**Depends on:** S08. **Area:** Workflow `results/`, artifact storage and candidate tables.

TODO:
- [ ] Parse schema-v2 `crops/reel.json` with its identifier definitions, and associate diagnostic information from `labels.json`, CSV, frame map and structured engine outcomes.
- [ ] Validate selected required slots, path safety, duplicate paths/identities, image decoding/dimensions/hashes, label count and missing/null values.
- [ ] Keep execution status distinct from quality status. A finished job with missing crops becomes review-blocked/incomplete, not enrollment-ready.
- [ ] Publish immutable result inventory/candidates, bounded thumbnails and source-provenance references; make importer retries idempotent across S3/DB boundaries.
- [ ] Add authorized attempt detail/candidate pagination/artifact download routes; short-lived URLs refer only to registered permitted artifacts.
- [ ] Test empty/partial/corrupt outputs, optimistic CSV completeness, missing rendered image, duplicate import, interrupted publication and unauthorized artifact access.

**Acceptance:** a valid small run appears as inspectable candidates; any missing/corrupt required image is a stable blocking finding.
**Not included:** editable algorithm YAML, huge embedded HTML as primary UI or the assumption that structural completeness proves correct physical association.

### S10 - Browse and process cloud runs from the desktop

**Outcome:** the first useful desktop feature works without a camera or enrollment permission.
**Depends on:** S09. **Area:** Desktop `pipeline/client.py`, controller and Runs panel.

TODO:
- [ ] Add workflow endpoint/project selection and reuse browser/PKCE sign-in; implement expiry/refresh or clear re-login using OS credential storage.
- [ ] Add Runs list, existing-S3 import action and run detail, including input status, profile, attempt history and capture-loss uncertainty.
- [ ] Wire Start Processing/Cancel/status polling to the real Workflow API; preserve command IDs across retries and cache only rebuildable cloud state.
- [ ] Show accurate queue/phase/error states, not guessed percentage from log lines; reconnect after network loss/app restart.
- [ ] Keep HTTP and disk work off the Qt thread, bound image/list memory, and allow cloud-only use without connected camera.
- [ ] Test slow/failing API, expired credentials, repeated clicks, restart, denied projects and UI responsiveness with a large fake run list.

**Acceptance:** operator imports/selects a run, launches processing and sees the same durable attempt after reopening the app.
**Not included:** enrollment controls, a local Rust sidecar or redesigning existing camera controls.

### S11 - Review physical labels and resolve findings

**Outcome:** operators can inspect evidence and accept/reject candidates without changing raw outputs.
**Depends on:** S10. **Area:** Desktop Review panel and Workflow `reviews/`.

TODO:
- [ ] Add virtualized candidate grid with full-resolution crop, selected slot names, source-frame neighbors and serial/QR provenance.
- [ ] Add filters/counts for missing image/QR/serial, duplicate identity/path, gap, direction/association concerns and review state.
- [ ] Persist accept/reject/needs-recapture decisions with reviewer, reason and expected revision; reject stale/concurrent overwrites.
- [ ] Apply the approved reference-data/QR validation policy server-side; preserve case-sensitive QR values and block unsupported stock/profile mismatches.
- [ ] Expose reprocess as a new pinned attempt; old reviews stay attached to old outputs. No silent fallback image, lossy conversion or manual identity edit without new revision/provenance.
- [ ] Test neighbor-QR misassociation examples, first/last labels, missing frames, duplicate identities and stale review against supplied ground truth.

**Acceptance:** a known bad association or missing required slot cannot be accepted as an enrollment-ready plan merely because the engine says complete.
**Not included:** profile authoring UI, Vlink-alias fallback without explicit approval, OCR or broad scientific re-tuning.

### S12 - Select target positions and freeze approval

**Outcome:** an immutable plan precisely defines what will be written and where.
**Depends on:** S04, S11. **Area:** Workflow `enrollment/` planning plus desktop target/approval UI.

TODO:
- [ ] Expose authorized APID Collection/Reel pickers via the server-owned adapter; persist UUIDs and confirm Team, mutable Reel state and expected composition.
- [ ] Implement the S00 mapping from candidate scan index/reference ordinal to explicit positive APID position; show reverse-feed and range interpretation for confirmation.
- [ ] Reject duplicate/occupied/unresolved positions and required-label gaps by policy; never silently compact, append or reverse positions.
- [ ] Validate every selected crop and identity/review revision server-side; freeze artifact hashes, member definitions, target context and explicit indexing into the approval digest.
- [ ] Persist plan/rows and approving subject transactionally; enforce idempotent approval commands and stale-revision rejection.
- [ ] Test changed crop, serial, position, Team, Reel or indexing after approval; each requires a new approved plan rather than mutation underneath enrollment.

**Acceptance:** the same approval produces the same plan ID/digest; a different approved payload cannot reuse it, and no APID mutation occurs during offline validation/planning.
**Not included:** Collection/Reel creation, enrollment writes or an invented remote dry-run/extract endpoint with no side effects.

### S13 - Enroll one approved pilot label

**Outcome:** the first product enrollment is one real, reviewed write with durable receipts.
**Depends on:** S12. **Area:** Workflow enrollment row worker and desktop Pilot action.

TODO:
- [ ] Add plan/row claim with exclusive target-Reel ownership, intent-before-send checkpoints and idempotent Start Pilot command.
- [ ] Read only frozen approved crops and verify bytes/context immediately before use; checkpoint each slot's extraction ID with content digest.
- [ ] Create the Label with all named DUST/TEXT/QR members, its final explicit position and explicit wire indexing through the S04 adapter.
- [ ] Validate returned Reel/position/live identifiers/values/indexing and persist crop-to-fingerprint-to-Label/member receipt before marking verified.
- [ ] Show an explicit real-write confirmation and result. Network ambiguity or crash becomes `reconciling`/operator action, not a blind resubmission; automated recovery comes in S14.
- [ ] Test double-click, denied target, malformed response, partial extraction, lost response and crash before receipt persistence; assert pilot cannot expand into a batch.

**Acceptance:** one approved candidate is traceable to one validated server result at its final position; uncertainty never appears as success.
**Not included:** bulk concurrency, deleting a pilot for re-enrollment, or production writes.

### S14 - Reconcile interrupted and ambiguous enrollment rows

**Outcome:** resuming cannot silently duplicate or alter the intended enrollment.
**Depends on:** S13. **Area:** Workflow recovery/reconciliation and fault-injection tests.

TODO:
- [ ] Implement deterministic recovery for each boundary: before extraction, after slot receipt, before create, after possible remote commit and before local result commit.
- [ ] Reuse persisted fingerprints only within verified server retention/context semantics; if necessary, re-extract the identical approved crop without changing intent.
- [ ] Reconcile APID's conditional same-Team/Reel/position DUST result; compare receipts, live membership and value/indexing rules. Counts alone do not prove DUST image identity.
- [ ] Classify network uncertainty, 401, 403, 429, scan/content failure and DUST/Vlink/position conflict; retry only unchanged safe intents with bounded backoff/jitter.
- [ ] Fence stale owners and serialize token renewal; an expired lease does not prove that an earlier HTTP request failed to commit.
- [ ] Test commit-then-disconnect, two workers, stale lease, compatible replay, different-Reel/position conflict, bound/transferred Label and persistent authorization failure.

**Acceptance:** injected lost-success responses resolve the original compatible result; conflicts/unknowns pause instead of creating a replacement or moving positions.
**Not included:** a universal exactly-once promise, treating every 409 as success or general APID idempotency-key support.

### S15 - Enroll the remainder with bounded concurrency

**Outcome:** approved Reels progress without requiring one click per label.
**Depends on:** S14. **Area:** Workflow batch scheduler and desktop Enroll panel.

TODO:
- [ ] Schedule only frozen, approved, incomplete rows; skip the verified pilot and completed rows without re-extracting them.
- [ ] Set low initial label concurrency and a separate extraction cap; apply fleet/Team limits, rate-limit feedback and fair scheduling.
- [ ] Implement start remainder, pause, resume and stop scheduling; allow in-flight writes to checkpoint before reporting paused/cancelled.
- [ ] Expose durable totals by verified/pending/reconciling/error state, per-row diagnostics and retry eligibility; completed percentage excludes uncertain rows.
- [ ] Require reauthorization/current valid plan context on commands; desktop disconnect does not cancel already-approved cloud work.
- [ ] Test a mixed-success batch, repeated Start/Resume, worker restart, 429 storm, revoked credentials, two operators and pause during in-flight writes.

**Acceptance:** a small approved batch completes/reports partial state correctly, and resume neither changes positions nor re-enrolls verified rows.
**Not included:** unbounded fan-out, auto-enroll after upload or cancellation that claims to roll back remote writes.

### S16 - Reconcile the Reel and export results

**Outcome:** completion is proven against APID, not inferred from a local counter.
**Depends on:** S15. **Area:** Workflow report job and desktop Results panel.

TODO:
- [ ] Fetch authoritative Reel detail and compare each approved position with receipt-backed Label/member IDs, live TEXT/QR values, indexing and allowed state.
- [ ] Surface missing/unexpected positions, duplicate warnings, changed membership and archived/transferred/bound state; do not settle a mismatch by changing remote data.
- [ ] Mark a plan completed only after reconciliation passes; otherwise persist partial/needs-attention with explicit reasons and last verified timestamp.
- [ ] Persist versioned JSON/CSV reports in S3, including input/profile/approval digests and returned IDs but no keys/tokens/presigned URL secrets.
- [ ] Add Results table, APID links and authorized export; neutralize CSV formulas and escape UI/report content.
- [ ] Test equal totals with wrong positions, post-enrollment remote changes, incomplete DUST receipts and interrupted report publication.

**Acceptance:** report identifies a position/member mismatch even when aggregate counts match; a report can be reconstructed without the original desktop.
**Not included:** automated Label repair, movement/binding or editing previously enrolled plans.

### S17 - Seal new captures safely

**Outcome:** the hardware app produces a trustworthy input inventory without blocking acquisition.
**Depends on:** S01. **Area:** Desktop `capture/writer.py`, `capture/runner.py`, `pipeline/seal.py`.

TODO:
- [ ] Assign stable local run ID and capture metadata before frames arrive; preserve current naming/layout and camera recipes.
- [ ] Write temporary image files then atomically rename; ignore unfinished files in discovery and record write failures.
- [ ] Replace timed-join-as-success with a real flush/stopped barrier, including relevant QR jobs; expose timeout/interrupted states.
- [ ] Record ordered files, encoded-byte digests, timestamps/settings and saved/dropped/failed counters without blocking the acquisition callback.
- [ ] Atomically publish a sealed capture manifest; add explicit recovery inventory for a crashed unsealed run and safe path/symlink validation.
- [ ] Test simulator capture, slow disk, queue overflow, disk full, Unicode paths, shutdown timeout and crash between file/manifest writes; retain existing golden camera tests.

**Acceptance:** sealed manifests enumerate only complete exact files; unfinished or uncertain capture cannot silently become ready to process.
**Not included:** changing trigger behavior, burst-to-label assumptions, stitching or cloud uploads.

### S18 - Add checksum-aware signed uploads and completion

**Outcome:** the backend can prove every new capture byte reached immutable cloud storage.
**Depends on:** S07. **Area:** Workflow storage/upload API and the chosen upload-signing implementation.

TODO:
- [ ] Add authorized upload-batch and complete-upload routes bound to one registered run/input inventory; keep one authority for key allocation and signing.
- [ ] Define safe canonical keys, signed checksum headers/receipts and exact size validation; use pinned versions or conditional immutable writes so outstanding URLs cannot alter sealed inputs.
- [ ] Mint bounded windows of at most the supported batch size; renew expired URLs without changing object identity. Existing same bytes may be acknowledged; conflicting bytes must be rejected.
- [ ] Reuse S07 inventory verification and DB/outbox commit behavior; do not trust the client saying uploaded or treat ETag as a universal content hash.
- [ ] Keep legacy prefixes/import behavior explicit; do not change existing capture uploads' namespace silently or grant broad read/write privileges.
- [ ] Test same-size different bytes, larger stale object, missing file, expired URL, duplicate completion, post-seal overwrite and forbidden key/project.

**Acceptance:** `raw_verified` is impossible while any expected object is absent, unverified or mutable outside its pinned identity.
**Not included:** per-byte multipart resume for ordinary small frames, a public bucket or auto-start on arbitrary S3 events.

### S19 - Persist and resume desktop uploads

**Outcome:** a station restart does not force a complete re-upload or hide corruption.
**Depends on:** S10, S17, S18. **Area:** Desktop `pipeline/local_store.py`, `pipeline/uploads.py` and upload UI.

TODO:
- [ ] Add SQLite migrations for local run registration, per-file digest/version receipt, retry state and persisted command IDs; separate authoritative local capture data from cloud cache.
- [ ] Stream sealed frame bytes through bounded workers and just-in-time URL batches; checkpoint after validated responses and preserve request/header separation for S3.
- [ ] On restart, reconcile unconfirmed transfers with backend receipts instead of assuming an interrupted PUT either succeeded or failed.
- [ ] Refresh operator credentials/URLs safely, retry transient errors with backoff, and implement pause/cancel between files without deleting local raw evidence.
- [ ] Show file/byte progress, failed-file reasons and server-verified completion; never label folder existence or larger remote size as success.
- [ ] Test >500 files, >1 listing page, process kill after remote PUT/before local receipt, changed local input, URL/token expiry, offline startup and UI responsiveness.

**Acceptance:** upload resumes at verified object boundaries; changed/corrupt data fails visibly and duplicate completion still produces one verified input.
**Not included:** removing local raw files automatically or processing an actively changing capture folder.

### S20 - Connect fresh capture to the complete operator journey

**Outcome:** an operator completes a new run without command-line tools.
**Depends on:** S16, S19. **Area:** Desktop run coordinator and existing workflow routes.

TODO:
- [ ] Add New Run dialog with authorized project/profile, printed reel display metadata and capture requirements; preserve capture-only/offline operation.
- [ ] Wire captured -> sealed -> upload -> verified -> process using existing IDs; distinguish local run from cloud run registration and attempt history without duplicating entities.
- [ ] Add optional auto-process only after verified upload; enrollment always requires review, frozen target/positions and real-write confirmation.
- [ ] Make every stage's actionable error lead to the appropriate retry/review screen; reconnect to the same run after app restart.
- [ ] Keep capture priority while older jobs run in the cloud; show that closing the UI does not undo cloud work or receipts.
- [ ] Demonstrate a simulator/synthetic journey and an approved real small-run journey including offline capture and later upload.

**Acceptance:** one new run reaches a reconciled result through the UI with unchanged raw evidence and no clid/local shell dependency.
**Not included:** rescan segment merging, automatic partial-Reel approval or a new hardware UI.

### S21 - Package and upgrade the Windows application

**Outcome:** the supported station can install/run/upgrade without a developer environment.
**Depends on:** S20. **Area:** Desktop packaging/release.

TODO:
- [ ] Extend existing PyInstaller/Inno configuration for new modules/credential backend; do not bundle clid or a Windows stitcher.
- [ ] Preserve Vimba runtime/licensing preflight and camera-optional cloud review; verify Unicode user paths and documented supported Windows versions.
- [ ] Add signed artifact/release verification with the approved signing owner; pin dependency/runtime versions and collect notices/SBOM.
- [ ] Test install/upgrade with populated SQLite/cache/run folders; migrate safely and define supported rollback behavior without destroying data.
- [ ] Include diagnostic export with secret redaction and a version/context header, plus a safe feature-disable path.
- [ ] Run clean-machine smoke tests with/without camera/runtime and record the full small-run installer demo.

**Acceptance:** the signed installer works on a clean supported station and an upgrade preserves active run/upload/enrollment references.
**Not included:** fleet auto-update service, cross-platform camera support or destructive rollback migrations.

### S22 - Prepare production operations and capacity

**Outcome:** the service is operable and recoverable, not merely deployable.
**Depends on:** S16, S19. **Area:** Workflow/platform/release.

TODO:
- [ ] Promote tested infrastructure definitions to the approved environment, with project-scoped IAM, private encrypted S3, backend TLS and enrollment secrets separated from algorithm jobs.
- [ ] Verify bucket retention/removal protection, PostgreSQL backup/PITR and recoverable approved artifacts; define raw/output/report/scratch retention and explicit cleanup eligibility.
- [ ] Add queue-age/job-heartbeat/upload-failure/auth-failure/reconciliation alarms and searchable run/attempt/plan/request correlation IDs; no secret-bearing logs.
- [ ] Exercise service-account rotation, token expiry, project entitlement changes and job/row lease recovery without broadening machine permissions.
- [ ] Measure p50/p95 upload/process/extract/enroll times and peak memory/disk on representative loads; set admission limits, backpressure, quotas and agreed operational budgets.
- [ ] Document rollback, stuck-job/reconciliation triage, volume cleanup, operator support and on-call ownership; run a backup restore drill.

**Acceptance:** owners can detect a stuck run, restore authoritative state and explain its current disposition without access to the original station.
**Not included:** adding another message broker, cluster platform or multi-region deployment without measured need. Baseline isolation/logging/limits were already required in earlier slices.

### S23 - Run integrated recovery and security rehearsals

**Outcome:** the complete release survives realistic interruptions without false completion.
**Depends on:** S21, S22. **Area:** cross-component acceptance harness and release candidate.

TODO:
- [ ] Kill/restart the desktop during writing, upload, review and enrollment display; verify no unsealed input or stale approval advances.
- [ ] Kill scheduler/worker or the stitcher runtime at each persistence boundary, including unavailable scratch storage and partial artifact publication.
- [ ] Inject APID commit-then-disconnect, extraction failure, prolonged 429, revoked credentials and duplicate operators; prove unchanged intents and conflict handling.
- [ ] Attempt cross-project artifact access, unauthorized approval, forged Team/Reel/image choice, path escape and secret leakage in diagnostic/CSV export.
- [ ] Test installer upgrade with an active upload/plan plus service/schema version compatibility; reject unsupported contract versions explicitly.
- [ ] Record each scenario's observed DB/S3/APID state and recovery instructions; resolve all release-blocking failures before physical pilot approval.

**Acceptance:** no tested failure loses raw evidence, silently changes positions, bypasses approval or marks uncertain APID state completed.
**Not included:** marking untested scenarios passed because individual unit tests were green.

### S24 - Complete the controlled physical-Reel pilot

**Outcome:** a release decision backed by real hardware, association quality and reconciliation evidence.
**Depends on:** S23. **Area:** operators, algorithm owner, product and platform.

TODO:
- [ ] Obtain explicit approval for the named real run, environment, Team/Reel, indexing, expected count/range and retention; a staging rehearsal is not implicit permission for production writes.
- [ ] Capture/upload/process representative stock and review correctness against independent physical label/QR/shield ground truth, including first/last labels and supported direction cases.
- [ ] Approve the exact output/position plan; run one real pilot label and validate its receipt before enrolling the remainder.
- [ ] Independently reconcile every intended position/member, explain exclusions/conflicts, and perform the approved physical DUST verification procedure where available.
- [ ] Record actual turnaround/resources/error rate, operator usability issues and recovery evidence against the S00 acceptance policy; make remaining limitations visible.
- [ ] Obtain operator/algorithm/platform/product sign-off and decide release, restricted pilot extension or blocked rollout. Archive evidence and open follow-up tickets.

**Acceptance:** correctness and recovery evidence supports the release decision; a green process or matching total count alone is insufficient.
**Not included:** expanding profiles, fleet size, partial-Reel policy or automation beyond the pilot's approved scope.

## 6. Demonstrable release gates

| Gate | Required slices | What we can demonstrate | Still disabled |
|---|---|---|---|
| G0 - Contract proof | S00-S02, S04 | Approved small raw run -> correct crops -> direct nonproduction APID receipt | Product enrollment, production writes |
| G1 - Cloud review alpha | S00-S11 | Existing S3 run -> verified processing -> desktop evidence review | Product enrollment until approval/row worker |
| G2 - Existing-S3 enrollment beta | S00-S16 | Review -> frozen plan -> pilot -> recoverable batch -> authoritative report | Production/fleet rollout; fresh-capture integration may be pending |
| G3 - Complete station workflow | S00-S20 | New capture -> verified upload -> process -> review -> enrollment -> report | Production release until release gates |
| G4 - Release candidate | S00-S23 | Signed installer + production-ready ops + integrated failure/security evidence | Physical production pilot until explicit authorization |
| G5 - Controlled release | S00-S24 | Real-Reel evidence and named sign-offs | Any scope not explicitly qualified |

These gates refine the earlier M0-M5 milestones. S04 is the early M0 proof; the ingest and existing-S3 work lanes intentionally overlap instead of making every milestone a strictly serial implementation block.

## 7. Follow-up backlog - deliberately not part of the first build

- [ ] F01: qualified nested-burst adapter. Preserve capture discontinuities; never infer one burst equals one label.
- [ ] F02: additional real-stock/profile combinations beyond S00, including new required DUST-slot sets. General keyed support and qualification of the selected pilot profile are already mandatory; each additional profile needs separate qualification.
- [ ] F03: interrupted/rescanned segment merging and explicit partial-Reel policy with stable physical positions.
- [ ] F04: Vlink alias enrichment, only after data-equivalence and authorization policy are approved.
- [ ] F05: automatic Collection/Reel creation. Requires server-supported idempotency or an explicitly supervised ambiguous-create resolution workflow; names are not unique.
- [ ] F06: local Windows stitcher runner or Windows-origin APID enrollment if the selected product requirements change.
- [ ] F07: SSE/progressive fine-grained engine events, retained-volume optimization and fleet updates, driven by measured need.
- [ ] F08: profile-tuning UI, automatic acceptance/enrollment or operator identity federation beyond the initial Google/workflow and service-account boundary.

A feature moves into the initial scope only with an explicit decision, revised dependency/acceptance criteria and any new security/scientific qualification work. Do not hide it inside another slice.

## 8. Before coding the first slice

- [ ] Review and approve the proposed architecture and initial scope in this document.
- [ ] Complete S00's decision sheet, especially required DUST slots, serial source, physical positions, indexing and operated cloud runtime.
- [ ] Confirm repositories/owners and where approved non-secret fixture copies will live.
- [ ] Select S01 as the first code slice after S00; do not start the entire service/UI in one change.
- [ ] Queue S02, S05 and S17 after S01 as independent engine, backend and desktop tasks; run S04 early once the engine/fixtures are trustworthy.

For each future slice handoff, record: `owner`, `status`, `dependencies passed`, `scope`, `PR(s)`, `test evidence`, `demo`, `known limitations`, `reviewer`. All current implementation statuses remain **not started**.
