# S00 — Pilot scope and access decisions

**Status:** Tauri/web UI with retained Python capture confirmed; S00 completion remains blocked on the other decisions and access below.
**Branch:** `slice/s00-pilot-scope`
**Backlog:** [S00 in IMPLEMENTATION_SLICES.md](../IMPLEMENTATION_SLICES.md#s00---lock-the-pilot-scope-and-unblock-access)

This sheet makes the first slice's open decisions explicit. It does not approve
application implementation, scientific label associations, infrastructure
changes, or live enrollment. S01 remains blocked until S00 acceptance passes.

## Confirmed workspace setup

- Private planning repository: [PintoGideon/label-enrollment-app](https://github.com/PintoGideon/label-enrollment-app).
- The initial documentation baseline is on `main`; S00 work is isolated on this branch.
- `reference/` remains local and ignored. Do not add it as tracked files or a submodule.
- User confirmed Tauri + bundled web UI with the existing Python camera/capture engine retained as a local helper. Replace the Qt presentation layer, not the hardware engine.
- Reuse the requested Rust stitcher in the proposed cloud pipeline; call APID directly without bundling `clid`. A Python capture helper is not a local stitcher or enrollment CLI.
- Creating this repository does **not** decide whether it will also host the Workflow backend.

## Required decisions

**D01a is confirmed; all other entries remain pending**, not defaults that an implementation may silently accept.
For each decision, record the selected value, approving owner, date, and evidence
or a link to the decision. Keep credentials and private fixture contents out of Git.

| ID | Decision | Proposal or information needed | Approval / evidence |
|---|---|---|---|
| D01a | Desktop shell and capture reuse | Tauri + bundled web UI; retain the existing Python capture engine as a supervised helper. New web controls/native IPC are in scope; no camera-driver/trigger rewrite. Tauri v2 is the implementation target. | Confirmed by user, 2026-09-15 EDT, in this planning conversation |
| D01b | Web framework/tooling | Proposed: React + TypeScript + Vite, local bundled assets and typed native adapters. Confirm before scaffolding; this does not imply a browser-hosted product. | Pending |
| D01c | Processing/enrollment origin | Proposed: cloud Rust stitcher and direct-APID cloud worker. Confirm whether enrollment HTTP instead must originate on Windows; never put APID credentials in the renderer or camera helper. | Pending |
| D02 | Repositories, runtime, compute, and ownership | New Tauri desktop is planned in this repository. Confirm Workflow home and TypeScript runtime, capture-helper upstream package/build/distribution arrangement and owner, backend/platform owners, and operated EKS versus Batch/ECS. Record nonproduction service endpoints without secrets. | Pending |
| D03 | Pilot label identity | Choose one profile and its version, exact required named DUST slots, QR-to-serial authority, and approved reference-data version/digest. Do not assume every shield crop should enroll. | Pending |
| D04 | Physical positions and indexing | Define scan-order/reference-order/APID-position mapping, direction/reverse feed, starting position, and first/last-label handling. Select explicit APID indexing (`none` = verify-only; `default` = identifiable). Confirm a no-gap/no-renumbering policy and that required-label failures block approval. | Pending |
| D05 | Approved fixtures and ground truth | Supply an approved small flat-stream raw run and representative edge cases, with independently checked label/QR/shield associations. Record permitted storage location, immutable identity, reviewer, and usage restrictions. Use supplied copies outside protected `manifests/`; do not read/change that directory. | Pending |
| D06 | Nonproduction access and write permission | Arrange a Team-scoped service account via an approved secret manager, existing disposable Collection/Reel UUIDs, and allowlisted S3 prefixes. Record approved APID/AuthD environments, access owner, and explicit permitted test writes, including fingerprint extraction. No production credentials/data are needed. | Pending |
| D07 | Station, packaging, capacity, and retention | Confirm Windows/camera/driver targets, WebView2 online/offline provisioning and patch ownership, Python-helper/Vimba distribution permissions, real-hardware checks, workload/throughput, and raw/result/checkpoint/approval/receipt retention. | Pending |

## Confirmed decision log

**2026-09-15 EDT — user:** “Yes lets do a tauri + web ui ?” in response to the explicit proposal to retain the Python capture engine as a local helper. This confirms D01a only. React/tooling, deployment, scope/data/access and release approvals are not inferred from that message.

Updated design consequences: native Rust owns helper supervision, operator credentials, cloud HTTP and the SQLite transfer journal; Python owns capture/sealing. IPC and helper packaging are new work, not already delivered by `labeltron-cli`. The old `PLAN.md` remains superseded; its clid/local-stitcher architecture is not restored.

## Current proposed pilot boundaries

These summarize the design for review; unresolved required decisions above remain blockers.

- Existing-S3 workflow first: import -> process -> review -> approve -> enroll -> report.
- One qualified profile, flat continuous streams, and one selected processing attempt per enrollment plan.
- Existing APID Collection and Reel selected by UUID; no automatic destination creation.
- Freeze crop hashes, identities, physical positions, destination context, and explicit indexing before enrollment.
- No automatic enrollment, silent gaps/renumbering, local Windows stitcher, camera-driver/trigger rewrite, or multi-segment Reel merging. Reimplementing supported capture controls in the web UI is now in scope.
- After S01, prove the Tauri shell/fake bridge in S10a and packaged simulator/helper in S17a before integrating hardware. Capture sealing and verified uploads retain their parallel lane; full installer qualification remains S21.

## Completion checklist

- [x] D01a: Tauri/web UI with retained Python capture helper selected by the user; implementation unstarted.
- [ ] D01b: frontend framework/tooling confirmed.
- [ ] D01c: cloud versus Windows-origin enrollment decision approved.
- [ ] D02: Workflow home/runtime, helper upstream/distribution, compute, service environments, and responsible owners approved.
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

**Acceptance record:** D01a confirmed; all remaining required decisions/access pending. No Tauri scaffold/native build, helper protocol implementation, or live S3/APID/compute/hardware checks have been performed as part of this slice.
