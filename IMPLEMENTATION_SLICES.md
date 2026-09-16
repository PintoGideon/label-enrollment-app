# Labeltron Enrollment - Implementation Slices and TODOs

**Status: parent backlog proposed; S01a local groundwork is in progress.** A Go/pnpm service with PostgreSQL pooling/migrations/readiness exists; its temporary smoke harness is in a separate local-only sibling folder, not the project. No parent acceptance gate is complete. Only evidenced child work and explicitly confirmed decisions may be checked. Repository-review findings are not implementation evidence.

Architecture: [SYSTEM_DESIGN.md](SYSTEM_DESIGN.md).
Detailed design: [DESKTOP_APP_PLAN.md](DESKTOP_APP_PLAN.md).
Source baseline: [SOURCES.md](SOURCES.md).
Identity decision: [AuthD login and accountable human ownership](plans/authentication-boundaries.md).

This document is the execution-order and TODO reference. The other documents explain the architecture; they are not competing task trackers.

### Next implementation increment — S01a list authorized projects

**[One endpoint and one client method](plans/S01a-authenticated-projects.md):**
AuthD-verified `GET /pipeline/v1/projects`, membership-filtered SQL and
`ListProjects`. Base `23df567`; suggested branch `slice/s01a-project-list`.
No feature code yet. Detail/readiness and other S05 work follow separately.

All integration proof lives in `../label-enrollment-harness/`; only focused
unit/contract tests accompany application code. Keep planning/relocation separate
from the feature PR. The linked ticket owns scope and acceptance; broader
S00/S01/S05 gates and foundation migration/CI follow-ups remain open.

## 1. How we will build

1. **For each thin feature: backend -> API -> nonvisual frontend/client integration -> scripted proof -> UI.** This user-selected order supersedes the earlier shell-first proof sequence.
2. Prove domain/state changes with unit tests, expose a validated API, wire a reusable typed client, then demonstrate the flow through scripts before adding its screen. Do not put essential behavior only in UI handlers.
3. Deliver the **existing-S3 path** first: import -> process -> review -> approve -> enroll -> report. Repeat the same backend-first cycle per capability; do not build one giant backend before testing anything.
4. Develop headless capture/sealing and upload APIs in parallel where dependencies allow. The Python helper and native command core must be scriptable without a Tauri window.
5. Keep slices independently reviewable and production enrollment disabled until explicit release/field gates pass. Existing APID/stitcher code is not proof that the new Workflow backend is implemented.

A slice should normally fit in a few focused engineering days and one or a few small PRs. If refinement reveals more than about three days of implementation, split it into child tickets before coding. Native builds, infrastructure access and scientific qualification are uncertain; these are sizing guidelines, not delivery promises.

### Current local test-tooling policy

All setup/integration harnesses, fixture generators, drivers and reports belong in
`../label-enrollment-harness/`, outside this repository and pnpm workspace.
Ordinary source-adjacent unit/contract tests remain; process/database/issuer
orchestration stays external. Harnesses call the actual service/public client,
with no copied backend or reverse production/CI dependency. Keep harness changes
and broad planning updates separate from feature PRs. Record concise commands,
outcomes and limits in review evidence; full logs belong in the harness folder.

### Required delivery order within a feature

| Phase | Deliverable | Gate before the next phase |
|---|---|---|
| B - Backend | Domain logic, durable state/migrations, jobs and adapters for this capability | Focused unit/component tests, failure semantics and ownership established |
| A - API | Versioned HTTP routes or typed local IPC commands with authorization, validation and stable errors | Contract/integration tests against a running service or real helper protocol |
| C - Frontend integration, no screens | Reusable typed API/native client, state mapping, reconnect/retry/cancel behavior | Same client/command core can run through a test-only headless harness |
| T - Scripted proof | Repeatable acceptance script exercises that client and actual API/state boundary | Assertions pass; nonzero exit on failure; secret-free evidence records what is real versus fake |
| U - UI | Tauri/web controls and evidence display over the proven client | UI smoke/error/accessibility tests plus regression of the same scripted flow |

B and A have tests from the start; T is an additional end-to-end gate, not the first time we test. For backend-only work such as checkpoint repair, use the same backend/API-or-process/script gates and omit inapplicable UI work explicitly.

**Example: run registry.** Persist run/command state -> expose create/list/detail routes -> implement a nonvisual run client -> script create, repeat the command, restart, retrieve the same run, and reject another project -> only then render the Runs screen.

Planned acceptance scripts must:

- Start/use the actual Workflow process and isolated PostgreSQL for durable-state scenarios; reach features through production HTTP/IPC clients, not direct DB mutation or a fake implementation of the new backend.
- Use synthetic fixtures and controlled object-store/runner/APID/auth adapters in normal CI. Label these external dependencies as fakes; passing them does not qualify live S3, real stitching, APID or hardware.
- Assert returned IDs/digests, persisted state and recovery, not just exit 0 or HTTP 200. Cover authorization, malformed input, duplicate commands, timeouts, partial results and relevant process restarts.
- Keep tokens/private data out of logs and evidence. A test-only harness is not a shipped clid dependency, generic native shell command or camera-control HTTP server.
- Require separate approved nonproduction credentials/targets and explicit opt-in for live writes, including extraction. Scripted approvals may be synthesized only for isolated synthetic fixtures; passing tests never auto-approves real label associations or production enrollment.

The S01a local PostgreSQL proof has passed outside the repository; authenticated project/API acceptance is still pending. The real-service/state/client requirements above continue to apply. See the readiness summary in [README.md](README.md#development-status).

### Initial scope, subject to S00 approval

- **Selected:** Tauri v2 + bundled web UI, retaining the existing Python capture engine as a local helper. React/TypeScript/Vite is proposed, pending framework confirmation. The Workflow backend runtime is Go (user confirmed 2026-09-15).
- **Selected identity/ownership:** one AuthD login; the verified human remains responsible. The target cloud executor uses a separate APID-audience service token. Client/audience registration, action-permission lookup, deployment and upload-auth transition remain qualification work under S00/S05/S10a/S18.
- Cloud Rust stitcher; cloud worker makes direct APID HTTP calls; no clid binary.
- One approved label profile and flat continuous streams first.
- One capture run / selected processing attempt per enrollment plan.
- Existing APID Collection/Reel selected by UUID for the first release.
- Explicit indexing and physical-position policy; never inherit API defaults silently.
- Required-label failures block plan approval. No silent fallback, skipped/re-numbered labels or automatic enrollment.
- Polling/phase progress first; no WebSocket/SSE infrastructure required.
- New web capture controls are in scope; rewriting camera drivers/trigger recipes is not. No algorithm rewrite, local Windows stitcher or multi-segment Reel merging.

## 2. Ownership and code boundaries

| Area | Implementation home | Notes |
|---|---|---|
| Desktop | New Tauri/web UI in this repository; proposed `apps/desktop/` | Web presentation; Rust-owned commands, credentials, helper supervision, cloud HTTP and SQLite upload journal |
| Capture helper | Approved `dustid/labeltron-two` working branch/package based on `jhodges/wininstaller`; distribution/owner confirmed in S00 | Reuse Qt-free core, simulator and preflight; add headless protocol/sealing; preserve existing CLI/Qt regression clients |
| Engine | `dustid/labeltron-two-stitcher` | Rust algorithm, wrapper, checkpoints and output contracts |
| Workflow | New Go backend; runtime confirmed 2026-09-15, repository/location still an S00 decision; S01a foundation under `apps/workflow/` | One Go module for HTTP API, scheduler, importer and enrollment worker |
| Contracts/tests | Proposed `packages/contracts/`, versioned schemas and safe fixtures | Cloud contracts and native/Python IPC; same identities/errors tested by Go, TypeScript, Rust and Python |
| Storage/compute | Approved cloud infrastructure | One upload-signing authority; adapt the existing upload service or implement its successor route deliberately |
| APID | Existing HTTP API | No server change needed for supervised enrollment into an existing Reel |

pnpm manages root tasks and future web packages under `apps/*` and `packages/*`. The backend's `package.json` only invokes Go tools; Go dependencies remain in `go.mod`/`go.sum`. No Turborepo, TypeScript backend or frontend scaffold is introduced. The initial Go client proves backend HTTP plumbing, not S10a's later native/web-client gate.

Suggested Workflow module boundaries under `apps/workflow/internal/`: `auth/`, `runs/`, `storage/`, `jobs/`, `results/`, `reviews/`, `enrollment/`, `apid/`. These are Go packages in one module, not separate microservices. Introduce tables/routes as their slices need them; do not scaffold every future module in the first PR.

The checkouts are now at `../label-enrollment-app-tests/reference/`, outside the app and harness modules. They remain study material, never runtime imports, vendored code or submodules in this repository. The user has separately authorized creation of this private planning repository and the S00 branch. Application implementation still requires the slice gates; use pinned packages/explicit working branches in the chosen repositories, not edits to those reference checkouts.

## 3. Slice index and dependencies

**S00 is in decision review**: desktop/capture reuse, delivery order, Go orchestration, pnpm workspace tooling and the AuthD/human-ownership model are confirmed; other prerequisites remain pending. See the [decision sheet](plans/S00-pilot-scope.md). The build kickoff started bounded local groundwork in [S01a](plans/S01a-backend-foundation.md); its Go service/PostgreSQL persistence checkpoint is implemented and locally proven, but signed identity and project APIs remain pending. All full parent gates below remain unpassed. Dependencies mean acceptance, not merely code existence; this groundwork does not authorize pilot-dependent or live-write work.

Keep the 25 parent IDs stable. Child gates separate backend/API/client/script readiness from UI completion; a downstream backend depends on the former, not on a screen. A parent passes only after its final child and inherited prerequisites pass. Parent numbers identify scope, not a mandatory serial execution order.

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
| S10 | Tauri desktop can browse a cloud run and start/watch processing | S10b | Desktop |
| S11 | Operator reviews images, identities and exceptions | S11b | Desktop / Workflow |
| S12 | Destination/positions selected; exact enrollment plan approved | S12b | Desktop / Workflow |
| S13 | One approved pilot label enrolled with a durable receipt | S13b | Desktop / Workflow |
| S14 | Lost API responses and interrupted rows reconcile safely, script-proven | S13a | Workflow |
| S15 | Bounded bulk enrollment with pause/resume | S15b | Desktop / Workflow |
| S16 | Per-position reconciliation and downloadable final report | S16b | Desktop / Workflow |
| S17 | Python helper produces atomic captures and a sealed manifest | S17b | Capture / desktop |
| S18 | Checksum-aware upload API and verified completion | S07 | Workflow / storage |
| S19 | Desktop uploads resume safely after restart | S19b | Desktop |
| S20 | Web capture controls enter the complete workflow without scripts | S20b | Desktop / Workflow |
| S21 | Signed Tauri/helper installer and upgrade/rollback validation | S21b | Desktop / release |
| S22 | Production infrastructure, capacity, backups and operations ready | S16a, S19a | Workflow / platform |
| S23 | Integrated failure/security rehearsal passes | S21, S22 | Cross-cutting |
| S24 | Controlled physical-Reel pilot and release decision | S23 | Operations / product / algorithm |

### Backend/script and UI child gates

Each row is separately reviewable; split B/A/C/T work further if it exceeds roughly three implementation days. An `a` backend/script gate is not a claim of UI completion. S17/S21 are explicit headless/release exceptions to the `a`/`b` split.

| ID | Deliverable / acceptance evidence | Depends on |
|---|---|---|
| S10a | Nonvisual native auth/Workflow client and frontend state adapter; scripts prove import/start/cancel/poll/reconnect through S09 APIs, with no Tauri window | S09 |
| S10b | Minimal Tauri shell, restricted command/window surface, bundled Runs UI over S10a, Windows/WebView2 and UI failure tests; completes S10 | S10a |
| S11a | Review persistence/authorization API, nonvisual review client, scripted stale-revision/QC/association tests; no screens | S09 |
| S11b | Review grid, filters and evidence inspection over the proven review client; completes S11 | S10, S11a |
| S12a | Target/position/approval backend and API, client and scripted immutable-plan proof with synthetic human-review decisions; no enrollment writes | S04, S11a |
| S12b | Target picker/position confirmation/approval UI over S12a; completes S12 | S11, S12a |
| S13a | Durable pilot worker and API, client and script proof of one approved row/receipt; fake APID in CI, explicit opt-in for real nonproduction writes | S12a |
| S13b | Real-write confirmation/Pilot UI over the proven command; completes S13 | S12, S13a |
| S15a | Bulk scheduler/API, client and scripted pause/restart/recovery proof; no UI | S14 |
| S15b | Enroll progress/pause/resume/conflict UI over S15a; completes S15 | S13, S15a |
| S16a | Reconciliation/report backend and API, client and scripted per-position/export assertions; no UI | S15a |
| S16b | Results/export UI over S16a; completes S16 | S15, S16a |
| S17a | Headless Python helper API, UI-independent Rust supervisor/client, packaged Windows simulator script proof including start/status/stop/EOF/backpressure; never claim sealing yet | S01 |
| S17b | Atomic files, writer/QR barrier, inventory and crash recovery through that client/protocol; scripted simulator/golden evidence; completes S17 | S17a |
| S19a | Native upload journal/engine, command client and scripts for sealed input/verified transfer/restart through S18; no UI | S10a, S17, S18 |
| S19b | Upload UI over S19a with responsiveness/failure tests; completes S19 | S10, S19a |
| S20a | Nonvisual capture-to-report client coordination and full scripted synthetic journey through helper, upload and Workflow APIs, with opt-in real qualification | S16a, S17, S19a |
| S20b | Capture/preview/settings controls and complete Tauri journey over the proven clients; offline/close/reload/owner tests; completes S20 | S16, S19, S20a |
| S21a | Tauri Windows bundle with pinned headless helper/resources, WebView2 and Vimba prerequisite handling; clean-machine/offline provisioning evidence | S20 |
| S21b | Signing, helper/host version checks, old-app data import, upgrade/rollback and diagnostic validation; completes S21 | S21a |

### Work lanes

```text
 BACKEND / API / NONVISUAL CLIENT / SCRIPT LANE (no UI prerequisite)

 S00 -> S01
 S01 -> S05 -> S06 -> S07
 S01 -> S02 -> S03           S02 -> S04 (opt-in APID proof)
 S02 + S03 + S07 ----------> S08 -> S09 -> S11a
 S04 + S11a ---------------> S12a -> S13a -> S14 -> S15a -> S16a
 S01 -> S17a -> S17b -> S17
 S09 -> S10a                 S07 -> S18
 S10a + S17 + S18 ---------> S19a
 S16a + S17 + S19a --------> S20a (scripted capture-to-report)
 S16a + S19a --------------> S22 (operations)

 UI LANE (each screen waits for its own script gate)

 S10a -> S10b -> S10
 S10 + S11a ---------------> S11b -> S11
 S11 + S12a ---------------> S12b -> S12
 S12 + S13a ---------------> S13b -> S13
 S13 + S15a ---------------> S15b -> S15
 S15 + S16a ---------------> S16b -> S16
 S10 + S19a ---------------> S19b -> S19
 S16 + S19 + S20a ---------> S20b -> S20 -> S21a -> S21b -> S21
 S21 + S22 ----------------> S23 -> S24
```

The parent and child dependency tables are authoritative if the overview diagram omits a dependency. S04 is an opt-in nonproduction technical probe; it does not unlock unattended/product enrollment. That only starts after review and durable intent exist in S12-S14.

## 4. Common definition of done

Every implementation slice must satisfy all of these, in addition to its own TODOs:

- [ ] Scope, owner, affected repo/branch and acceptance cases agreed before coding.
- [ ] B/A/C/T/U order followed where applicable; backend/API/client/script gate accepted before its UI work. Record any inapplicable layer explicitly.
- [ ] Focused unit/contract tests and headless proof recorded; all integration harnesses/fixtures/reports live in `../label-enrollment-harness/`. Record concise review evidence without importing harness machinery into the feature PR. Local results are not shared CI/release acceptance; real network/hardware writes remain separately opt-in.
- [ ] Schema changes have migrations/compatibility tests where applicable; no hand-edited generated APID artifacts.
- [ ] Authorization, bounded resources, secret redaction and safe path handling included where first introduced.
- [ ] State-changing operations have durable intent and defined retry/cancel behavior; no false exactly-once claims.
- [ ] Existing capture/simulator behavior still passes where affected; no UI-thread disk/network work.
- [ ] Feature can be disabled safely; rollback does not delete raw captures, approvals or receipts.
- [ ] Script command, fixture/version, expected assertions, nonzero failure behavior and secret-free evidence recorded; distinguish actual backend/DB, mocked dependencies and separately qualified live services.
- [ ] Human review accepted; update only the completed slice's checklist/status.

A later reliability/security slice is a system-level rehearsal, not permission to defer essential safeguards until release.

## 5. Detailed slice checklists

### S00 - Lock the pilot scope and unblock access

**Outcome:** one explicit decision sheet, not assumptions hidden in code.
**Depends on:** none. **Area:** product, algorithm owner and platform owner.
**Decision review:** [plans/S00-pilot-scope.md](plans/S00-pilot-scope.md), on `slice/s00-pilot-scope`. Desktop shell/core reuse, delivery order and the Go Workflow runtime are confirmed; remaining required decisions and access are pending.

TODO:
- [x] Select Tauri + bundled web UI while retaining the Python capture engine as a local helper (user confirmed; no implementation implied).
- [x] Use backend -> API -> nonvisual frontend/client integration -> scripted proof -> UI for each feature (user confirmed; no tests/implementation implied).
- [ ] Confirm frontend framework/tooling proposal and cloud enrollment worker versus Windows-origin enrollment; record these separately from the shell and delivery-order decisions.
- [x] Confirm the Workflow runtime: Go (user confirmed 2026-09-15; no implementation completeness implied).
- [x] Use pnpm for workspace tasks/future web dependencies; retain Go modules for backend dependencies (user confirmed 2026-09-15).
- [x] Record one AuthD login and accountable human ownership, with separate service execution (S00 D10; design agreement only).
- [ ] Record issuer/native-client/Workflow-audience setup and owners for current org/Team permissions and upload-auth transition. Keep runtime qualification in S05/S10a/S13/S18/S19; do not make those implementations prerequisites of S00.
- [ ] Confirm the Workflow repository home, capture-helper upstream/distribution/owners, operated EKS versus Batch/ECS, nonproduction endpoints and owners.
- [ ] Choose the initial profile, required DUST slots, serial authority and reference-data version.
- [ ] Define scan-order/reference-order/APID-position mapping, including reverse feed and endpoint labels; define indexing and no-gap policy.
- [ ] Obtain an approved small raw run, representative edge cases and independently checked label-to-QR-to-shield ground truth. Use supplied copies outside protected `manifests/`; do not read/change that directory.
- [ ] Arrange a nonproduction Team-scoped service account, pre-created disposable Reel(s), approved S3 prefixes and explicit permission for the test writes.
- [ ] Record supported Windows/camera and WebView2 environment, helper/runtime distribution permissions, target workload, retention, and which checks require real hardware.

**Acceptance:** required scope/identity/position/indexing decisions are approved and prerequisite fixture/environment access is available. An unresolved required item keeps S00 blocked; merely naming its owner does not complete the slice. Normal development needs no production credentials/data.
**Not included:** implementation, dependency installation, real enrollment or infrastructure deployment.

### S01 - Define executable contracts and a safe test harness

**Outcome:** all components agree on input/output, identity and errors before integration.
**Depends on:** S00. **Area:** shared contracts and chosen Workflow test package.
**Local groundwork:** [S01a](plans/S01a-backend-foundation.md) records the limited Go/pnpm foundation begun after the build kickoff. Its probe/client tests do not complete the contracts or prerequisites below.

TODO:
- [ ] Define minimal versioned `CaptureManifest`, `ProcessingAttempt`, `ResultManifest`, `LabelCandidate`, `EnrollmentPlan`, row receipt and event/error schemas.
- [ ] Define native/Python IPC envelopes: version/session handshake, bounded JSON Lines, request/run IDs, stable errors, start/status/stop, event ordering, preview handles, timeout/unknown outcome and shutdown semantics; raw pixels are not JSON payloads.
- [ ] Distinguish run ID, attempt ID, candidate ID, plan ID, physical position, Reel UUID and command ID; specify enums and legal transitions.
- [ ] Specify canonical digest inputs and normalization: content/version identity, QR exactness, TEXT semantics, positions, target context and indexing; exclude signed URLs/tokens.
- [ ] Add synthetic/golden fixtures: valid and incomplete schema-v2 Reel documents, multiple named DUST slots, duplicate identity/path, unsafe path, reverse mapping and partial capture.
- [ ] Add fake native/helper, HTTP/object-store/runner adapters plus deterministic clock/fault injection, including malformed/oversized IPC, helper restart and delayed start responses. Mocks must not be mistaken for cloud, camera or scientific qualification.
- [ ] Establish focused CI and acceptance-script conventions: explicit fixtures, actual local service/DB boundary, bounded waits, assertions, exit codes and redacted evidence. Define a UI-independent production client/command seam usable by scripts; normal CI requires no camera, AWS/APID key or protected reference files.

**Acceptance:** Go/TypeScript/Rust/Python consumers interpret shared golden identities/digests consistently; invalid fixtures and incompatible helper messages fail with stable machine codes.
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
**Current placement:** production migrations/database/auth code belongs here; temporary PostgreSQL setup and API proof drivers belong in the separate local test folder, not new project harness packages.
**Next bounded increment:** [S01a project listing](plans/S01a-authenticated-projects.md)
implements only the local project-list auth/API/client proof. Project detail and
readiness changes follow separately. Its completion
does not pass S05's inherited S00/S01 or remaining context/permission gates.

TODO:
- [x] Implement startup, configuration, pgx pooling, Goose SQL migrations and sqlc-generated database readiness queries (S01a local proof). Generated-code checks run with unit/build CI; overall authorized-API readiness remains pending and temporary DB provisioning stays outside this repo.
- [ ] Validate AuthD signatures, configured issuer/Workflow audience and lifetime using maintained JWT/JWKS libraries; bound trusted HTTPS key retrieval/cache/refresh and fail closed. No new identity provider or auth bypass.
- [ ] Map verified `(issuer, subject)` to PostgreSQL project membership for read-only discovery; recognize human/service principals without treating a service token as human approval.
- [ ] Add project list/detail, bounded UUID-keyset pagination, deterministic capture/process/review/enroll roles, non-disclosing 404s, server request IDs and redacted structured logging; implement truthful project-API readiness.
- [ ] Before enabling commands, qualify current org/Team/action permission lookup and server-owned S3/APID context. Org claims, readable Team lists and client-supplied Team/bucket/image choices are not sufficient authority. Keep this outside the read-only S01a increment.
- [ ] Test missing/expired/wrong-audience tokens, a disallowed project and attempted destination escalation; fail closed outside explicitly isolated tests.
- [ ] Define the backend API/worker roles in one codebase and establish a nonproduction smoke deployment or local isolated equivalent.

- [ ] Add a nonvisual project/auth client and script that starts/targets the real isolated Workflow service, verifies readiness/migrations, lists allowed projects and rejects missing/expired/forged/wrong-project credentials. Test auth fixtures must not bypass production checks.

**Acceptance:** the script proves allowed access and denial through the running API; a forged subject/project/context cannot cause privileged work. This is the first new backend/API capability, not an already available service.
**Not included:** all pipeline features, AWS master keys in the desktop, Better Auth setup or new operator sign-up flows.

### S06 - Register runs and persist commands/events

**Outcome:** creating a run twice or restarting the service does not lose or duplicate its identity.
**Depends on:** S05. **Area:** Workflow `runs/`, PostgreSQL, outbox/events.

TODO:
- [ ] Add runs and minimal command/outbox/event tables with project ownership, input revision, origin, capture metadata and actual storage locator. Derive responsible human and initiator from verified AuthD issuer/subject at cloud registration; no submitted owner IDs or user tokens in rows/events/jobs.
- [ ] Reauthorize run commands using S05's qualified action/context policy. Preserve owner attribution across app closure, renewal and actions by another authorized person; offline captures gain an authenticated cloud owner only at registration. Test spoofed ownership and cross-principal actions; ownership transfer is deferred.
- [ ] Add idempotent `POST /pipeline/v1/runs`, authorized paginated list/detail, revision checks and stable command IDs.
- [ ] Return the original result for a repeated identical command; reject reuse of a command ID with changed semantics.
- [ ] Make state transition + durable event/outbox writes transactional; add safe event pagination/cursors for polling.
- [ ] Define intermediate/recovery states and timeouts instead of deriving completion from a local in-memory flag.
- [ ] Test concurrent duplicate submissions, restart, denied project access, stale revision and outbox replay.

- [ ] Add a headless run client and repeatable script: create -> replay same command -> retrieve/list -> restart backend -> retrieve same state; changed-command replay and cross-project reads fail. Runtime operations use the API, not DB writes.

**Acceptance:** scripted API/client proof shows one run ID, restart durability and no cross-project leakage before any Runs screen is implemented.
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

- [ ] Extend the typed import client and script to verify pagination, immutable input identity and denied prefixes through the actual Workflow API/DB with a controlled object store; live S3 qualification is separate.

**Acceptance:** script proves processing references a fixed verified frame set even if the old upload prefix later changes.
**Not included:** automatic processing on every S3 object-created event or generic recursive burst flattening.

### S08 - Run one cloud processing attempt

**Outcome:** an authorized run launches one bounded job, and its status survives service restart.
**Depends on:** S02, S03, S07. **Area:** Workflow `jobs/`, Engine staging wrapper and approved compute.

TODO:
- [ ] Add attempts/job leases and start/status/cancel routes; pin input digest, asset bundle, image digest and profile before enqueueing.
- [ ] Implement one runner adapter for the S00 runtime, including minimal nonproduction IAM, storage and central logging; do not implement EKS and Batch simultaneously.
- [ ] Use scheduler workload permissions for configured job submission/observation and separate stitcher IAM for approved S3 scopes. Jobs receive no human AuthD token or APID secret; the app needs no cluster credentials or arbitrary job YAML. Persist authorized initiating actor and runtime identity separately.
- [ ] Stage only inventory-referenced versions/bytes; use validated SDK parameters and collision-free scratch paths, not user-controlled shell strings.
- [ ] Make job identity deterministic per attempt; recover a create-call timeout by discovering that job before submitting anything else. Persist runtime IDs and fence stale callbacks.
- [ ] Watch actual runtime exit/conditions/heartbeat and bounded phase progress; configure CPU/memory/disk/deadline/fleet limits.
- [ ] Implement cancellation and exact-version checkpoint restoration or retained-volume reuse. If checkpoints cannot be verified, rerun in new scratch; never pretend an empty volume is resumable.
- [ ] Test duplicate starts, lost submit response, worker restart, OOM, timeout, cancellation, failed result upload and recovery after final-upload code cannot run.

- [ ] Extend the nonvisual processing client and script to start/poll/cancel/reconnect, inject ambiguous runner submission and restart the scheduler. Use a deterministic runner in CI and record separate opt-in cloud evidence.

**Acceptance:** scripted API/state proof shows one active runtime owner per attempt; a failed/killed job never succeeds merely because an output prefix exists.
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

- [ ] Add the candidate/artifact client and script the complete import -> process -> candidate/artifact flow, including a missing/corrupt required crop and unauthorized access. Run the real new backend; fake only declared external storage/runner dependencies.

**Acceptance:** the headless script proves a valid fixture becomes inspectable candidates and a missing/corrupt required image is a stable blocking finding. This API-ready gate precedes S10 client integration and Tauri screens; it is not live scientific qualification.
**Not included:** editable algorithm YAML, huge embedded HTML as primary UI or the assumption that structural completeness proves correct physical association.

### S10 - Browse and process cloud runs from the desktop

**Outcome:** the first useful Tauri desktop feature works without a camera, helper runtime or enrollment permission.
**Depends on:** S10b (inherits S10a and S09). **Area:** UI-independent native auth/Workflow services and frontend state adapter, then proposed `apps/desktop/` shell/screens.

TODO:
- [ ] S10a: implement the nonvisual frontend/native adapter over proven S09 APIs: allowlisted Workflow/project selection, AuthD system-browser/PKCE callback validation with registered native client and Workflow audience, renewal/re-login, OS credential storage, durable command IDs and reconnect/state mapping. Keep these modules usable from a headless test runner; no tokens in renderer/helper and no second Google login. Record live issuance/renewal qualification separately from synthetic proof.
- [ ] S10a: script import/start/poll/cancel/reconnect and denial/expiry/repeated-command cases through that same adapter and actual isolated API/DB. A fake UI adapter alone does not pass the gate.
- [ ] S10b, only after S10a passes: create the minimal Tauri/bundled UI, bind the tested command core, configure explicit custom-command/window permissions and CSP, and test denied generic shell/file/URL access plus Windows/WebView2 smoke installation.
- [ ] Add Runs list, existing-S3 import action and run detail, including input status, profile, attempt history and capture-loss uncertainty.
- [ ] Wire Start Processing/Cancel/status polling to the real Workflow API; preserve command IDs across retries and cache only rebuildable cloud state.
- [ ] Show accurate queue/phase/error states, not guessed percentage from log lines; reconnect after network loss/app restart.
- [ ] Keep HTTP/disk work in asynchronous native services, bound renderer list/image memory, and allow cloud-only use with missing helper/Vimba/camera. Render only bundled application code with native privileges.
- [ ] Test slow/failing API, expired credentials, repeated clicks, restart, denied projects and UI responsiveness with a large fake run list.

**Acceptance:** S10a's nonvisual client/script proof passes before S10b starts; the UI then reproduces the same import/process/reconnect outcomes without reimplementing business rules. Mock UI tests supplement, not replace, the actual API/script proof.
**Not included:** enrollment or camera controls (S20b), live capture, a local stitcher or browser-hosted deployment.

### S11 - Review physical labels and resolve findings

**Outcome:** operators can inspect evidence and accept/reject candidates without changing raw outputs.
**Depends on:** S11b; S11a depends on S09, while S11b depends on S10 and S11a. **Area:** Workflow `reviews/`, nonvisual review client, then Desktop Review screen.

TODO:
- [ ] S11a/B-A: persist accept/reject/needs-recapture decisions with verified reviewer issuer/subject, reason and expected revision; expose authorized review/reprocess APIs and reject stale/concurrent overwrites. Derive actors server-side and preserve the run's responsible owner.
- [ ] Apply approved reference-data/QR validation server-side; preserve exact QR values, reject unsupported stock/profile mismatches, and attach reviews to immutable outputs. No fallback image, lossy conversion or identity edit without new revision/provenance.
- [ ] S11a/C-T: implement the review client and script candidate inspection, synthetic review decisions, stale revisions, missing slots and reprocess invalidation through the APIs; test neighbor-QR/endpoint/duplicate examples against approved ground truth. Never synthesize approvals for real runs from structural test success.
- [ ] S11b/U: add virtualized grid, full crops, selected slots, source neighbors, serial/QR provenance and exception filters/counts over that proven client; add UI error/reload tests.

**Acceptance:** scripted server-side proof blocks known bad/missing-slot candidates before the screen is built; S11b displays the same persisted decisions and failures.
**Not included:** profile authoring UI, Vlink-alias fallback without explicit approval, OCR or broad scientific re-tuning.

### S12 - Select target positions and freeze approval

**Outcome:** an immutable plan precisely defines what will be written and where.
**Depends on:** S12b; S12a depends on S04 and S11a, while S12b depends on S11 and S12a. **Area:** Workflow planning/API, typed client/script, then target/approval UI.

TODO:
- [ ] S12a/B-A: expose authorized Collection/Reel catalog and planning/approval endpoints via the server-owned adapter; persist UUIDs and validate Team, mutable Reel state and expected composition.
- [ ] Implement the S00 mapping from candidate scan index/reference ordinal to explicit positive APID position; return reverse-feed/range interpretation for explicit client confirmation.
- [ ] Reject duplicate/occupied/unresolved positions and required-label gaps by policy; never silently compact, append or reverse positions.
- [ ] Validate every selected crop and identity/review revision server-side; freeze artifact hashes, member definitions, target context and explicit indexing into the approval digest.
- [ ] Persist plan/rows and verified human approving issuer/subject transactionally; enforce idempotent approval commands and stale-revision rejection. Recheck current action/context authority; a submitted actor header or service token cannot impersonate a human approver.
- [ ] S12a/C-T: implement the nonvisual target/approval client and script synthetic review -> plan -> validate -> approve; repeated intent yields the same ID/digest. Changed crop/identity/position/Team/Reel/indexing invalidates approval. Assert zero APID mutations from planning.
- [ ] S12b/U: implement destination pickers, position/range display and exact-plan approval confirmation over that tested client; keep approval explicit and server-authoritative.

**Acceptance:** S12a passes independently of screens; S12b reproduces the same immutable-plan behavior. A different payload cannot reuse approval, and offline validation/planning performs no APID mutation.
**Not included:** Collection/Reel creation, enrollment writes or an invented remote dry-run/extract endpoint with no side effects.

### S13 - Enroll one approved pilot label

**Outcome:** the first product enrollment is one real, reviewed write with durable receipts.
**Depends on:** S13b; S13a depends on S12a, while S13b depends on S12 and S13a. **Area:** Workflow pilot worker/API, typed client/script, then Pilot UI.

TODO:
- [ ] Add plan/row claim with exclusive target-Reel ownership, intent-before-send checkpoints and idempotent Start Pilot command.
- [ ] Persist verified human enrollment requester and approved-plan reference; identify the AuthD service executor on attempts/receipts separately. Check qualified permissions, destination and cancellation/revocation policy before dispatch; queue durable intent without user tokens. Define/test continued approved work after UI logout or closure.
- [ ] Read only frozen approved crops and verify bytes/context immediately before use; checkpoint each slot's extraction ID with content digest.
- [ ] Create the Label with all named DUST/TEXT/QR members, its final explicit position and explicit wire indexing through the S04 adapter.
- [ ] Validate returned Reel/position/live identifiers/values/indexing and persist crop-to-fingerprint-to-Label/member receipt before marking verified.
- [ ] S13a/A-C-T: expose the explicit pilot command/status API and nonvisual client; script one approved synthetic row through real Workflow state and fake APID. Test repeated commands, denied target, malformed/partial/lost responses and crash before receipt; the pilot cannot expand into a batch. Live nonproduction writes require separate approved input/target and explicit opt-in.
- [ ] Treat ambiguity as `reconciling`/operator action, never blind resubmission; automated recovery follows in S14.
- [ ] S13b/U: add real-write confirmation and receipt/conflict display using the proven client; no bypass of the server's frozen-plan/intent gate.

**Acceptance:** S13a proves row/receipt and unknown-outcome behavior before Pilot UI exists; S13b displays it without hiding uncertainty or changing the approved position.
**Not included:** bulk concurrency, deleting a pilot for re-enrollment, or production writes.

### S14 - Reconcile interrupted and ambiguous enrollment rows

**Outcome:** resuming cannot silently duplicate or alter the intended enrollment.
**Depends on:** S13a, not Pilot UI completion. **Area:** Workflow recovery/reconciliation and scripted fault-injection harness.

TODO:
- [ ] Implement deterministic recovery for each boundary: before extraction, after slot receipt, before create, after possible remote commit and before local result commit.
- [ ] Reuse persisted fingerprints only within verified server retention/context semantics; if necessary, re-extract the identical approved crop without changing intent.
- [ ] Reconcile APID's conditional same-Team/Reel/position DUST result; compare receipts, live membership and value/indexing rules. Counts alone do not prove DUST image identity.
- [ ] Classify network uncertainty, 401, 403, 429, scan/content failure and DUST/Vlink/position conflict; retry only unchanged safe intents with bounded backoff/jitter.
- [ ] Fence stale owners and serialize token renewal; an expired lease does not prove that an earlier HTTP request failed to commit.
- [ ] Test commit-then-disconnect, two workers, stale lease, compatible replay, different-Reel/position conflict, bound/transferred Label and persistent authorization failure.

- [ ] Drive recovery/status through the production nonvisual client/API in a restartable script; assert persisted receipt/member identity after commit-then-disconnect, not merely unit-test mock calls.

**Acceptance:** scripted lost-success responses resolve the original compatible result without a UI; conflicts/unknowns pause instead of creating a replacement or moving positions.
**Not included:** a universal exactly-once promise, treating every 409 as success or general APID idempotency-key support.

### S15 - Enroll the remainder with bounded concurrency

**Outcome:** approved Reels progress without requiring one click per label.
**Depends on:** S15b; S15a depends on S14, while S15b depends on S13 and S15a. **Area:** Workflow batch/API, nonvisual client/scripts, then Enroll UI.

TODO:
- [ ] Schedule only frozen, approved, incomplete rows; skip the verified pilot and completed rows without re-extracting them.
- [ ] Set low initial label concurrency and a separate extraction cap; apply fleet/Team limits, rate-limit feedback and fair scheduling.
- [ ] Implement start remainder, pause, resume and stop scheduling; allow in-flight writes to checkpoint before reporting paused/cancelled.
- [ ] Expose durable totals by verified/pending/reconciling/error state, per-row diagnostics and retry eligibility; completed percentage excludes uncertain rows.
- [ ] Require reauthorization/current valid plan context on commands; desktop disconnect does not cancel already-approved cloud work.
- [ ] S15a/C-T: implement the batch client and script mixed success, repeated Start/Resume, worker restart, 429 storm, revoked credentials, two operators and pause during in-flight writes against actual Workflow state and declared fake APID.
- [ ] S15b/U: add progress, pause/resume and conflict controls over the script-proven client; do not infer verified progress from rendered rows.

**Acceptance:** S15a independently proves partial/resume behavior without changed positions or re-enrollment; S15b then reproduces it through the UI.
**Not included:** unbounded fan-out, auto-enroll after upload or cancellation that claims to roll back remote writes.

### S16 - Reconcile the Reel and export results

**Outcome:** completion is proven against APID, not inferred from a local counter.
**Depends on:** S16b; S16a depends on S15a, while S16b depends on S15 and S16a. **Area:** Workflow report/API, nonvisual client/scripts, then Results UI.

TODO:
- [ ] Fetch authoritative Reel detail and compare each approved position with receipt-backed Label/member IDs, live TEXT/QR values, indexing and allowed state.
- [ ] Surface missing/unexpected positions, duplicate warnings, changed membership and archived/transferred/bound state; do not settle a mismatch by changing remote data.
- [ ] Mark a plan completed only after reconciliation passes; otherwise persist partial/needs-attention with explicit reasons and last verified timestamp.
- [ ] Persist versioned JSON/CSV reports in S3, including input/profile/approval digests and returned IDs but no keys/tokens/presigned URL secrets.
- [ ] S16a/A-C-T: expose authorized reconciliation/report/export APIs and client; script equal totals with wrong positions, remote changes, incomplete DUST receipts and interrupted publication. Neutralize CSV formulas and assert exact report/receipt references, not aggregate counts.
- [ ] S16b/U: add Results table, APID links and export over the proven client; escape displayed/report content and show pending/conflict states.

**Acceptance:** S16a reconstructs the report and detects per-position mismatches without any desktop; S16b presents the same outcome. The complete backend approval/enrollment/report path is scriptable independently of its screens.
**Not included:** automated Label repair, movement/binding or editing previously enrolled plans.

### S17 - Seal new captures safely

**Outcome:** the supervised Python capture helper produces a trustworthy inventory without blocking acquisition.
**Depends on:** S17b (inherits S17a and S01); no Tauri shell dependency. **Area:** approved `labeltron-two` working branch `capture/{writer,runner,seal}.py`/`headless/` and UI-independent native supervisor/client later bound into Tauri. Do not change `reference/`.

TODO:
- [ ] S17a: wrap existing `BurstRunner`/`RunRequest`, typed capture callbacks, camera protocol/simulator, runtime and preflight with a headless versioned pipe protocol; stdout is protocol-only, stderr is redacted logs. No cloud credentials or upload worker in Python.
- [ ] S17a: add Rust fixed-executable supervision, handshake/version checks, single camera/run ownership, start-intent/status reconciliation, bounded messages/preview metadata, simultaneous pipe draining, stop/EOF/exit timeouts and Windows process cleanup. Never replay an unknown start blindly.
- [ ] S17a: build a pinned target-specific helper with PyInstaller and script simulator/preflight/start/status/stop through the same native supervisor/command core on Windows, without a Tauri window or user-installed Python. Keep CLI/Qt and golden tests passing. Tauri binding/UI checks follow at the UI gate, and missing helper/runtime must not block cloud review.
- [ ] S17b: assign stable local run ID and capture metadata before frames arrive; preserve current naming/layout and camera recipes.
- [ ] Write temporary image files then atomically rename; ignore unfinished files in discovery and record write failures.
- [ ] Replace timed-join-as-success with a real flush/stopped barrier, including relevant QR jobs; expose timeout/interrupted states.
- [ ] Record ordered files, encoded-byte digests, timestamps/settings and saved/dropped/failed counters without blocking the acquisition callback.
- [ ] Atomically publish a sealed capture manifest; add explicit recovery inventory for a crashed unsealed run and safe path/symlink validation.
- [ ] Test simulator capture, slow disk, queue overflow, disk full, Unicode paths, protocol mismatch, malformed/oversized output, helper/native crash, duplicate start, blocked stderr/stdout, EOF/stop timeout and crash between file/manifest writes; retain existing golden camera tests.

**Acceptance:** headless client/protocol scripts pass for S17a/S17b before capture controls are built. Stop/exit cannot fake a seal; manifests enumerate complete exact files and uncertain capture remains needs-attention.
**Not included:** changing trigger behavior, burst-to-label assumptions, capture web controls (S20b), stitching or cloud uploads.

### S18 - Add checksum-aware signed uploads and completion

**Outcome:** the backend can prove every new capture byte reached immutable cloud storage.
**Depends on:** S07. **Area:** Workflow storage/upload API and the chosen upload-signing implementation.

TODO:
- [ ] Add authorized upload-batch and complete-upload routes bound to one registered run/input inventory; keep one authority for key allocation and signing.
- [ ] Use the AuthD-authorized Workflow boundary as the signing entry point. Adapt or replace legacy Google-only upload authorization; do not assume it accepts AuthD tokens. Test one-login authorization, project/key denial and expiry before claiming complete capture/upload integration.
- [ ] Define safe canonical keys, signed checksum headers/receipts and exact size validation; use pinned versions or conditional immutable writes so outstanding URLs cannot alter sealed inputs.
- [ ] Mint bounded windows of at most the supported batch size; renew expired URLs without changing object identity. Existing same bytes may be acknowledged; conflicting bytes must be rejected.
- [ ] Reuse S07 inventory verification and DB/outbox commit behavior; do not trust the client saying uploaded or treat ETag as a universal content hash.
- [ ] Keep legacy prefixes/import behavior explicit; do not change existing capture uploads' namespace silently or grant broad read/write privileges.
- [ ] Test same-size different bytes, larger stale object, missing file, expired URL, duplicate completion, post-seal overwrite and forbidden key/project.

- [ ] Add a nonvisual upload-API client/driver and script batch signing, controlled object PUT, complete-upload, replay and checksum failure through actual Workflow APIs/DB. Reuse this contract/client in S19's native transfer integration.

**Acceptance:** scripted proof shows `raw_verified` is impossible while an expected object is absent, unverified or mutable outside its pinned identity.
**Not included:** per-byte multipart resume for ordinary small frames, a public bucket or auto-start on arbitrary S3 events.

### S19 - Persist and resume desktop uploads

**Outcome:** a station restart does not force a complete re-upload or hide corruption.
**Depends on:** S19b; S19a depends on S10a, S17 and S18, while S19b depends on S10 and S19a. **Area:** UI-independent native storage/transfer services and client/scripts, then web upload UI.

TODO:
- [ ] Add Rust-owned SQLite migrations for local run registration, per-file digest/version receipt, retry state and persisted command IDs; separate authoritative local capture data from cloud cache. Python publishes sealed manifests; neither helper nor renderer writes this DB.
- [ ] Stream sealed frame bytes through bounded workers and just-in-time URL batches; checkpoint after validated responses and preserve request/header separation for S3.
- [ ] On restart, reconcile unconfirmed transfers with backend receipts instead of assuming an interrupted PUT either succeeded or failed.
- [ ] Refresh operator credentials/URLs safely, retry transient errors with backoff, and implement pause/cancel between files without deleting local raw evidence.
- [ ] S19a/A-C-T: expose typed upload commands/state through the nonvisual client and script >500 files, pagination, kill after PUT/before local receipt, changed input, expiry and offline startup against actual native journal/Workflow APIs with controlled storage.
- [ ] S19b/U: show file/byte progress, failed-file reasons and server-verified completion over that client; test responsiveness. Folder existence or larger remote size is not success.

**Acceptance:** S19a proves verified-boundary resume and corruption/duplicate-completion behavior without UI; S19b displays those proven states.
**Not included:** removing local raw files automatically or processing an actively changing capture folder.

### S20 - Connect fresh capture to the complete operator journey

**Outcome:** an operator completes a new run through Tauri web controls without command-line tools.
**Depends on:** S20b (S16, S19, S20a); S20a depends on S16a, S17 and S19a and needs no UI. **Area:** nonvisual capture-to-report coordination/scripts, then web capture screens and native bindings.

TODO:
- [ ] S20a/C-T: compose the proven helper/upload/Workflow clients into a nonvisual run coordinator and script the full synthetic capture -> seal -> verified upload -> process -> review/approval fixture -> pilot/batch -> report flow. Use actual new backend/journals and declared fake external services; real association/write qualification is separately opt-in.
- [ ] S20a: script offline capture/later upload, start/stop/restart and pending/error transitions using the same command core that the UI will call. No runtime DB bypass, real automatic approval or new hardware recipes.
- [ ] S20b/U, after S20a passes: add web camera/preflight/settings/preview/mode/start/status/stop controls and New Run dialog over the proven coordinator; preserve supported camera behavior, loss counters and capture-only/offline operation.
- [ ] S20b: use bounded/downsampled preview cache and native-scoped opaque handles; no raw JSON/base64 firehose. Test saturation, renderer reload, owner exclusion and close/cancel/stop-and-flush without silently completing a run.
- [ ] Wire captured -> sealed -> upload -> verified -> process using existing IDs; distinguish local run from cloud run registration and attempt history without duplicating entities.
- [ ] Add optional auto-process only after verified upload; enrollment always requires review, frozen target/positions and real-write confirmation.
- [ ] Make every stage's actionable error lead to the appropriate retry/review screen; reconnect to the same run after app restart.
- [ ] Keep capture priority while older jobs run in the cloud; show that closing the UI does not undo cloud work or receipts.
- [ ] Demonstrate a simulator/synthetic journey and an approved real small-run journey including offline capture and later upload.

**Acceptance:** S20a proves the full nonvisual journey before S20b UI work; S20b then reproduces it through supported capture controls with unchanged raw evidence, no clid and no operator shell commands. CI synthetic evidence is not a substitute for the approved real pilot.
**Not included:** hardware-driver/trigger redesign, rescan segment merging or automatic partial-Reel approval.

### S21 - Package and upgrade the Windows application

**Outcome:** the supported station installs/runs/upgrades Tauri and the capture helper without user-installed Python, Node or Rust.
**Depends on:** S21b (inherits S21a and S20). **Area:** Tauri bundle and Python-helper packaging/release.

TODO:
- [ ] S21a: build a Tauri Windows installer (NSIS proposed; MSI only if required) with pinned PyInstaller headless helper via `bundle.externalBin`, correct target suffix/resources/native DLLs. Reuse licence/preflight findings from the old packaging, not the Inno UI installer. No clid or Windows stitcher.
- [ ] S21a: qualify WebView2 bootstrap/offline provisioning and patch ownership, plus Vimba prerequisites/licensing separately. Test cloud-only review with missing camera/Vimba and supported Unicode paths/Windows versions.
- [ ] S21b: sign/verify host, helper and installer as applicable with the approved signing owner; pin runtime/core/protocol versions and collect notices/SBOM. Reject helper/host version mismatch before camera commands.
- [ ] S21b: test existing Qt-app settings/capture import and install/upgrade with populated SQLite/cache/run folders. Preserve raw data, prevent concurrent camera ownership, migrate safely and define supported nondestructive rollback.
- [ ] Include diagnostic export with secret redaction and a version/context header, plus a safe feature-disable path.
- [ ] Run clean-Windows smoke tests without a developer toolchain, with/without WebView2/camera/Vimba, offline prerequisites as required, and packaged helper exit/restart; record the full small-run installer demo.

**Acceptance:** S21a/S21b pass; signed installer works on a clean supported station and an upgrade preserves active run/upload/enrollment references.
**Not included:** fleet auto-update service, cross-platform camera support or destructive rollback migrations.

### S22 - Prepare production operations and capacity

**Outcome:** the service is operable and recoverable, not merely deployable.
**Depends on:** S16a and S19a (script-proven backend/native engines, not their screens). **Area:** Workflow/platform/release.

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
- [ ] Kill/restart the webview, native host and Python helper independently during writing/upload/review; test EOF, orphan prevention, duplicate starts and preview backpressure. No unsealed input or stale approval advances; cloud enrollment remains independent of the renderer.
- [ ] Kill scheduler/worker or the stitcher runtime at each persistence boundary, including unavailable scratch storage and partial artifact publication.
- [ ] Inject APID commit-then-disconnect, extraction failure, prolonged 429, revoked credentials and duplicate operators; prove unchanged intents and conflict handling.
- [ ] Attempt cross-project access, unauthorized approval, forged Team/Reel/image choice, path escape and diagnostic/CSV secret leakage; test remote-origin/native-command denial, generic-shell attempts, compromised preview handles and token absence from renderer/helper.
- [ ] Test Tauri/helper upgrade with an active upload/plan plus WebView2, helper protocol and service/schema compatibility; reject unsupported versions explicitly and preserve the legacy capture data.
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
| G1-api - Headless cloud API proof | S09 and inherited prerequisites | Scripted import -> process -> candidates/artifacts through actual Workflow APIs; external fakes declared | UI acceptance, live/scientific qualification and enrollment |
| G1 - Cloud review alpha | S10, S11 and inherited script gates | Proven clients -> desktop evidence review | Product enrollment until approval/row worker |
| G2-api - Headless enrollment/report proof | S16a and inherited prerequisites | Scripted review -> frozen plan -> pilot -> recoverable batch -> per-position report; real writes only with explicit opt-in | UI acceptance, production/fleet rollout |
| G2 - Existing-S3 enrollment beta | S16 and inherited UI/script gates | Same proven flow usable through Tauri review/enrollment/report screens | Production/fleet rollout; fresh-capture integration may be pending |
| G3 - Complete station workflow | S00-S20 | New capture -> verified upload -> process -> review -> enrollment -> report | Production release until release gates |
| G4 - Release candidate | S00-S23 | Signed installer + production-ready ops + integrated failure/security evidence | Physical production pilot until explicit authorization |
| G5 - Controlled release | S00-S24 | Real-Reel evidence and named sign-offs | Any scope not explicitly qualified |

These gates refine M0-M5. S04 remains the early opt-in adapter proof, not evidence that the new Workflow service exists. Backend/script and UI gates are separate: API readiness does not imply UI acceptance, and UI work must not unlock otherwise untestable backend behavior.

## 7. Follow-up backlog - deliberately not part of the first build

- [ ] F01: qualified nested-burst adapter. Preserve capture discontinuities; never infer one burst equals one label.
- [ ] F02: additional real-stock/profile combinations beyond S00, including new required DUST-slot sets. General keyed support and qualification of the selected pilot profile are already mandatory; each additional profile needs separate qualification.
- [ ] F03: interrupted/rescanned segment merging and explicit partial-Reel policy with stable physical positions.
- [ ] F04: Vlink alias enrichment, only after data-equivalence and authorization policy are approved.
- [ ] F05: automatic Collection/Reel creation. Requires server-supported idempotency or an explicitly supervised ambiguous-create resolution workflow; names are not unique.
- [ ] F06: local Windows stitcher runner or Windows-origin APID enrollment if the selected product requirements change.
- [ ] F07: SSE/progressive fine-grained engine events, retained-volume optimization and fleet updates, driven by measured need.
- [ ] F08: profile-tuning UI, automatic acceptance/enrollment or additional identity-provider federation beyond the selected AuthD human/workload model.

A feature moves into the initial scope only with an explicit decision, revised dependency/acceptance criteria and any new security/scientific qualification work. Do not hide it inside another slice.

## 8. Next increment and remaining parent gates

- [x] Reconcile the architecture/backlog with the selected AuthD login and human ownership; preserve source-review/runtime limits.
- [ ] Implement the bounded [S01a project-list increment](plans/S01a-authenticated-projects.md) after foundation `23df567`: verifier -> API -> Go client -> isolated proof. No live environment or UI is needed for this increment.

- [ ] Review and approve the proposed architecture and initial scope in this document.
- [ ] Complete S00's decision sheet, especially required DUST slots, serial source, physical positions, indexing and operated cloud runtime.
- [ ] Confirm repositories/owners and where approved non-secret fixture copies will live.
- [ ] After S00, complete S01's broader contracts/fixtures beyond the bounded S01a groundwork; do not start the entire service/UI in one change.
- [ ] After S01, prioritize S05's backend/API/client/script proof; S02 engine work and S17a headless helper can proceed independently. Continue S06-S09 API proofs before S10a frontend-client integration and S10b Tauri UI. Run S04 only after its engine/fixture/access gates; do not introduce shell-first dependencies.

For each future slice handoff, record: `owner`, `status`, `dependencies passed`, `scope`, `PR(s)`, `test evidence`, `demo`, `known limitations`, `reviewer`. S01a's foundation is committed on `slice/s01a-backend-foundation`; authenticated project discovery is its next unimplemented increment, with suggested branch `slice/s01a-project-list`. No full parent gate has passed. S06 follows S05 acceptance, then S07 verified import and S08/S09 processing/results according to their dependencies; existing migration/CI follow-ups remain tracked separately.
