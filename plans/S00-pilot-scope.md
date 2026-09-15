# S00 — Pilot scope and access decisions

**Status:** decision review started; completion blocked on the approvals and access below.
**Branch:** `slice/s00-pilot-scope`
**Backlog:** [S00 in IMPLEMENTATION_SLICES.md](../IMPLEMENTATION_SLICES.md#s00---lock-the-pilot-scope-and-unblock-access)

This sheet makes the first slice's open decisions explicit. It does not approve
application implementation, scientific label associations, infrastructure
changes, or live enrollment. S01 remains blocked until S00 acceptance passes.

## Confirmed workspace setup

- Private planning repository: [PintoGideon/label-enrollment-app](https://github.com/PintoGideon/label-enrollment-app).
- The initial documentation baseline is on `main`; S00 work is isolated on this branch.
- `reference/` remains local and ignored. Do not add it as tracked files or a submodule.
- Reuse the requested Labeltron capture app and Rust stitcher; call APID directly without bundling `clid`.
- Creating this repository does **not** decide whether it will also host the Workflow backend.

## Required decisions

All entries below are **pending**, not defaults that an implementation may silently accept.
For each decision, record the selected value, approving owner, date, and evidence
or a link to the decision. Keep credentials and private fixture contents out of Git.

| ID | Decision | Proposal or information needed | Approval / evidence |
|---|---|---|---|
| D01 | Operator experience and enrollment origin | Proposed: extend the Python/PyQt6 Windows app, process with the cloud Rust stitcher, and call APID from a cloud worker. Confirm whether requests instead must originate on Windows. | Pending |
| D02 | Workflow repository, runtime, compute, and ownership | Proposed: one TypeScript backend for API, scheduler, importer, and enrollment-worker roles. Confirm this repository versus another home, backend/platform owners, and already operated EKS versus Batch/ECS. Record nonproduction service endpoints without secrets. | Pending |
| D03 | Pilot label identity | Choose one profile and its version, exact required named DUST slots, QR-to-serial authority, and approved reference-data version/digest. Do not assume every shield crop should enroll. | Pending |
| D04 | Physical positions and indexing | Define scan-order/reference-order/APID-position mapping, direction/reverse feed, starting position, and first/last-label handling. Select explicit APID indexing (`none` = verify-only; `default` = identifiable). Confirm a no-gap/no-renumbering policy and that required-label failures block approval. | Pending |
| D05 | Approved fixtures and ground truth | Supply an approved small flat-stream raw run and representative edge cases, with independently checked label/QR/shield associations. Record permitted storage location, immutable identity, reviewer, and usage restrictions. Use supplied copies outside protected `manifests/`; do not read/change that directory. | Pending |
| D06 | Nonproduction access and write permission | Arrange a Team-scoped service account via an approved secret manager, existing disposable Collection/Reel UUIDs, and allowlisted S3 prefixes. Record approved APID/AuthD environments, access owner, and explicit permitted test writes, including fingerprint extraction. No production credentials/data are needed. | Pending |
| D07 | Station, capacity, and retention | Confirm supported Windows/camera/driver environment, available real-hardware checks, representative and maximum run size, throughput expectations, and retention for raw data, results, checkpoints, approvals, and receipts. | Pending |

## Current proposed pilot boundaries

These summarize the design for review; unresolved required decisions above remain blockers.

- Existing-S3 workflow first: import -> process -> review -> approve -> enroll -> report.
- One qualified profile, flat continuous streams, and one selected processing attempt per enrollment plan.
- Existing APID Collection and Reel selected by UUID; no automatic destination creation.
- Freeze crop hashes, identities, physical positions, destination context, and explicit indexing before enrollment.
- No automatic enrollment, silent gaps/renumbering, local Windows stitcher, camera rewrite, or multi-segment Reel merging.
- Capture sealing and verified uploads can follow their parallel lane after S01 prerequisites pass.

## Completion checklist

- [ ] D01: deployment/enrollment-origin decision approved.
- [ ] D02: Workflow home/runtime, compute, service environments, and responsible owners approved.
- [ ] D03: profile, DUST slots, serial authority, and reference-data version approved.
- [ ] D04: direction/order/position rules, indexing, and no-gap behavior approved.
- [ ] D05: approved fixture copies and independent ground truth actually available.
- [ ] D06: nonproduction access, disposable destinations, S3 scope, and explicit test-write permission actually available.
- [ ] D07: supported station, workload, retention, and hardware verification expectations recorded and approved.
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

**Acceptance record:** pending. No live S3, APID, compute, or hardware checks have
been performed as part of this slice.
