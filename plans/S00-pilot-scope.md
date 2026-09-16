# S00 — Pilot scope and access decisions

**Status:** Tauri/web UI with retained Python capture, backend-first/script-before-UI delivery, Go orchestration and pnpm workspace tooling confirmed; S00 completion remains blocked on the other decisions and access below.
**Branch:** `slice/s00-pilot-scope`
**Backlog:** [S00 in IMPLEMENTATION_SLICES.md](../IMPLEMENTATION_SLICES.md#s00---lock-the-pilot-scope-and-unblock-access)

This sheet makes the first slice's open decisions explicit. It does not approve
scientific label associations, infrastructure changes or live enrollment. The
later build kickoff started bounded local groundwork in [S01a](S01a-backend-foundation.md);
full S01/S05 acceptance still requires the unresolved S00 prerequisites.

## Confirmed workspace setup

- Private planning repository: [PintoGideon/label-enrollment-app](https://github.com/PintoGideon/label-enrollment-app).
- The initial documentation baseline is on `main`; S00 work is isolated on this branch.
- Reference checkouts now live outside the repository at `../label-enrollment-app-tests/reference/`; revisions were preserved and Git worktree pointers repaired. Do not add them as tracked files, submodules or runtime dependencies. Protected manifests remain uninspected.
- User confirmed Tauri + bundled web UI with the existing Python camera/capture engine retained as a local helper. Replace the Qt presentation layer, not the hardware engine.
- Reuse the requested Rust stitcher in the proposed cloud pipeline; call APID directly without bundling `clid`. A Python capture helper is not a local stitcher or enrollment CLI.
- Creating this repository does **not** decide whether it will also host the Workflow backend. The S01a Go foundation is developed under `apps/workflow/` here until that D02 item is settled.

## Required decisions

**D01a, D08, D09 and the D02 runtime language are confirmed; all other entries remain pending**, not defaults that an implementation may silently accept.
For each decision, record the selected value, approving owner, date, and evidence
or a link to the decision. Keep credentials and private fixture contents out of Git.

| ID | Decision | Proposal or information needed | Approval / evidence |
|---|---|---|---|
| D01a | Desktop shell and capture reuse | Tauri + bundled web UI; retain the existing Python capture engine as a supervised helper. New web controls/native IPC are in scope; no camera-driver/trigger rewrite. Tauri v2 is the implementation target. | Confirmed by user, 2026-09-15 EDT, in this planning conversation |
| D01b | Web framework/tooling | Proposed: React + TypeScript + Vite, local bundled assets and typed native adapters. Confirm before scaffolding; this does not imply a browser-hosted product. | Pending |
| D01c | Processing/enrollment origin | Proposed: cloud Rust stitcher and direct-APID cloud worker. Confirm whether enrollment HTTP instead must originate on Windows; never put APID credentials in the renderer or camera helper. | Pending |
| D02 | Repositories, runtime, compute, and ownership | New Tauri desktop is planned in this repository. **Workflow runtime: Go**, one module for the HTTP API, scheduler, importer and enrollment worker roles. Still confirm the Workflow repository home, capture-helper upstream package/build/distribution arrangement and owner, backend/platform owners, and operated EKS versus Batch/ECS. Record nonproduction service endpoints without secrets. | Runtime language: Go confirmed by user, 2026-09-15 EDT, in this planning conversation. All other D02 items: Pending |
| D03 | Pilot label identity | Choose one profile and its version, exact required named DUST slots, QR-to-serial authority, and approved reference-data version/digest. Do not assume every shield crop should enroll. | Pending |
| D04 | Physical positions and indexing | Define scan-order/reference-order/APID-position mapping, direction/reverse feed, starting position, and first/last-label handling. Select explicit APID indexing (`none` = verify-only; `default` = identifiable). Confirm a no-gap/no-renumbering policy and that required-label failures block approval. | Pending |
| D05 | Approved fixtures and ground truth | Supply an approved small flat-stream raw run and representative edge cases, with independently checked label/QR/shield associations. Record permitted storage location, immutable identity, reviewer, and usage restrictions. Use supplied copies outside protected `manifests/`; do not read/change that directory. | Pending |
| D06 | Nonproduction access and write permission | Arrange a Team-scoped service account via an approved secret manager, existing disposable Collection/Reel UUIDs, and allowlisted S3 prefixes. Record approved APID/AuthD environments, access owner, and explicit permitted test writes, including fingerprint extraction. No production credentials/data are needed. | Pending |
| D07 | Station, packaging, capacity, and retention | Confirm Windows/camera/driver targets, WebView2 online/offline provisioning and patch ownership, Python-helper/Vimba distribution permissions, real-hardware checks, workload/throughput, and raw/result/checkpoint/approval/receipt retention. | Pending |
| D08 | Delivery order per capability | Backend -> API -> nonvisual frontend/client integration -> scripted proof -> UI. Backend/API tests start immediately; scripts exercise the same production client/API or helper core before screens. Backend gates do not depend on UI completion. | Confirmed by user, 2026-09-15 EDT, in this planning conversation |
| D09 | Workspace package manager/task entry point | pnpm workspace for `apps/*` and future `packages/*`. The Go service has a task-only `package.json`; dependencies remain Go modules. No TypeScript backend, frontend framework or extra task runner is implied. | Confirmed by user, 2026-09-15 EDT: “Can we move towards pnpm workspace??” |

## Confirmed decision log

**2026-09-15 EDT — user:** “Yes lets do a tauri + web ui ?” in response to the explicit proposal to retain the Python capture engine as a local helper. This confirms D01a only. React/tooling, deployment, scope/data/access and release approvals are not inferred from that message.

Updated design consequences: native Rust owns helper supervision, operator credentials, cloud HTTP and the SQLite transfer journal; Python owns capture/sealing. IPC and helper packaging are new work, not already delivered by `labeltron-cli`. The old `PLAN.md` remains superseded; its clid/local-stitcher architecture is not restored.

**2026-09-15 EDT — user:** requested backend -> API -> frontend -> test via scripting -> UI. D08 records this per-feature delivery order; frontend before UI means nonvisual client/state integration. It supersedes starting S10a with a Tauri shell. The actual Workflow backend, API/state store and acceptance scripts still do not exist; existing APID/stitcher source is not readiness evidence for them.

**2026-09-15 EDT — user:** “Yes we are going to do go.” This confirms **Go** as the Workflow backend language/runtime, after review found the S01a foundation already implemented in Go while the specs still said TypeScript. It confirms only the D02 runtime language: the Workflow repository home, compute platform, owners and every other pending decision stay open. The foundation lives in `apps/workflow/` on `slice/s01a-backend-foundation`; that working location does not settle the repository-home question.

Updated design consequences: the backend is one Go module whose API, scheduler, importer and enrollment worker are packages, not services. `packages/contracts/` golden fixtures gain a Go consumer beside the TypeScript web UI, Rust host and Python helper. The TypeScript library choices in the original S01a sheet (Fastify, `pg`, JOSE) are replaced by maintained Go equivalents selected in S01a. The frontend proposal (D01b) is unchanged.

**2026-09-15 EDT — user:** requested a pnpm workspace (D09). Root commands now delegate to the Go toolchain; pnpm does not compile Go or replace `go.mod`. The installed pnpm 11.20.0 is pinned without a global upgrade. Frontend framework approval, native/Rust client gates and the remaining pilot decisions are unchanged.

**2026-09-15 EDT — development policy:** the user requested that temporary test/setup harnesses stay in a separate, uncommitted folder and that existing script/harness clutter be cleaned up. S01a now keeps these under `../label-enrollment-app-tests/harness/`, separate from the relocated `reference/` sibling. Ordinary Go unit tests stay with source; project builds and CI do not depend on local-only files. Formal project harnesses are deferred until requested. This placement policy does not complete any pending pilot/access gate or change D08's backend-before-UI testing order.

## Current proposed pilot boundaries

These summarize the design for review; unresolved required decisions above remain blockers.

- Existing-S3 workflow first: import -> process -> review -> approve -> enroll -> report.
- One qualified profile, flat continuous streams, and one selected processing attempt per enrollment plan.
- Existing APID Collection and Reel selected by UUID; no automatic destination creation.
- Freeze crop hashes, identities, physical positions, destination context, and explicit indexing before enrollment.
- No automatic enrollment, silent gaps/renumbering, local Windows stitcher, camera-driver/trigger rewrite, or multi-segment Reel merging. Reimplementing supported capture controls in the web UI is now in scope.
- After S01, prioritize S05's backend/API/client/script proof and continue through run/import/processing APIs. S17a proves the headless helper independently; S10a frontend integration waits for S09 API proof, and S10b UI waits for S10a scripts. Each later feature repeats this order; full installer qualification remains S21.

## Completion checklist

- [x] D01a: Tauri/web UI with retained Python capture helper selected by the user; implementation unstarted.
- [x] D08: backend/API/nonvisual-client/script/UI delivery order selected; no implementation or test success implied.
- [x] D09: pnpm workspace selected; Go remains the orchestration language.
- [ ] D01b: frontend framework/tooling confirmed.
- [ ] D01c: cloud versus Windows-origin enrollment decision approved.
- [x] D02 (runtime language): Go selected by the user for the Workflow backend; no implementation completeness implied.
- [ ] D02 (remaining): Workflow home, helper upstream/distribution, compute, service environments, and responsible owners approved.
- [ ] D03: profile, DUST slots, serial authority, and reference-data version approved.
- [ ] D04: direction/order/position rules, indexing, and no-gap behavior approved.
- [ ] D05: approved fixture copies and independent ground truth actually available.
- [ ] D06: nonproduction access, disposable destinations, S3 scope, and explicit test-write permission actually available.
- [ ] D07: station/WebView2/helper prerequisites, distribution permissions, workload, retention, and hardware verification expectations approved.
- [ ] Record acceptance evidence here and update only the corresponding completed S00 backlog items.

Naming an owner or proposing a value does not complete a decision. An unresolved
required item keeps S00 blocked. Do not mark S00 complete merely because this
branch is merged.

## Verification and next step

- Check each approved value against the [detailed design](../DESKTOP_APP_PLAN.md)
  and [system diagrams](../SYSTEM_DESIGN.md); reconcile any affected passages together.
- Preserve the source-review limits in [SOURCES.md](../SOURCES.md). Mocked fixtures
  do not establish scientific correctness or live-service compatibility.
- Keep reference checkouts, private raw/reference data, and credentials out of commits.
- After all S00 acceptance conditions pass, create a separate S01 contracts/test-harness branch.
  Live extraction/enrollment remains a later, explicitly authorized opt-in activity.

**Acceptance record:** D01a, D08, D09 and the D02 runtime language (Go) confirmed; all remaining required decisions/access pending. S01a has Go/pnpm startup, PostgreSQL project/membership migrations and local real-database evidence, not a completed Workflow domain API or project/PostgreSQL acceptance gate. No Tauri/native build, helper protocol or live S3/APID/compute/hardware check has been completed.
