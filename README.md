# Labeltron Enrollment

Planning and implementation workspace for an end-to-end Labeltron workflow:

```text
Capture -> verified S3 upload -> Rust stitching -> review -> approve -> APID enrollment -> reconcile/report
```

The selected desktop direction is **Tauri + a bundled web UI**, retaining the
existing Python camera/capture engine as a supervised local helper. Tauri v2
with React + TypeScript + Vite is the proposed implementation stack; the web
framework is still to be confirmed.

```text
Bundled web UI -> Tauri native Rust host -> Python capture helper -> camera
                         |
                         +-> cloud Workflow -> S3 / Rust stitcher / APID
```

The native host owns credentials, scoped commands and uploads; Python owns
capture/sealing. The proposed cloud pipeline reuses `dustid/labeltron-two-stitcher`
and direct APID HTTP calls. No `clid` or local Windows stitcher is bundled.

## Documents

- [Detailed design](DESKTOP_APP_PLAN.md)
- [System architecture and ASCII diagrams](SYSTEM_DESIGN.md)
- [Implementation slices, dependencies, and TODOs](IMPLEMENTATION_SLICES.md)
- [Source revisions, evidence, and verification limits](SOURCES.md)
- [Historical plan — superseded](PLAN.md)

## Development status

**The new Workflow backend is not ready: this repository currently contains only planning documents.**

| Component | Current state |
|---|---|
| Workflow service, `/pipeline/v1` API, durable job/approval/enrollment store | Not implemented |
| APID/AuthD | Existing code/contracts reviewed; target live environment and access not validated |
| Rust stitcher and Python capture core | Existing reusable implementations; identified hardening and integration remain |
| Headless clients/acceptance scripts, capture helper protocol, Tauri UI | Not implemented |

Application implementation has not started. The backlog begins with **S00:
lock the pilot scope and unblock access**. Required decisions and approved
nonproduction fixtures/access must be resolved before starting S01.
The [S00 decision sheet](plans/S00-pilot-scope.md) is being reviewed on
`slice/s00-pilot-scope`. Desktop-shell/capture reuse and backend-first delivery
are confirmed; remaining scope, framework, environment and access decisions
still block S00.

Use a separate branch for each slice. Keep tests, review evidence, and scope
changes with that slice; do not mark proposed or mocked behavior as validated.
Creating this repository does not yet decide where the Workflow backend will
live; that remains an S00 decision.

## Delivery order

Repeat for each small capability:

```text
Backend -> API -> nonvisual frontend/client integration -> scripted proof -> UI
```

The frontend stage means reusable client/state logic, not screens. Scripts
exercise the actual new API/DB or helper/command core through that client,
assert failure/restart behavior and declare external fakes. Unit/API tests run
from the start; UI is added only after its script gate passes. Normal tests use
synthetic fixtures; live writes need separate explicit nonproduction approval.

After S00 and S01 contracts, prioritize S05's backend/project API proof, then
run-registry/import/processing APIs. The Tauri shell is no longer the first
implementation target.

## Local references and sensitive data

`reference/` contains local, independently managed source checkouts for study.
It is excluded by `.gitignore` and is not part of this repository's commits or
GitHub content. Source provenance is recorded in `SOURCES.md` instead.

Do not force-add reference checkouts, credentials, raw captures, or private
fixture data. Follow each reference repository's instructions; in particular,
do not read or change the stitcher's protected `manifests/` directory.
