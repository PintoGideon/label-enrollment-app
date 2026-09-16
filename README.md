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
The cloud Workflow backend is a Go service; the runtime was confirmed on
2026-09-15. **pnpm manages workspace tasks and future web packages**, not Go
dependencies or the deployed backend runtime.

## Documents

- [Detailed design](DESKTOP_APP_PLAN.md)
- [System architecture and ASCII diagrams](SYSTEM_DESIGN.md)
- [Implementation slices, dependencies, and TODOs](IMPLEMENTATION_SLICES.md)
- [Source revisions, evidence, and verification limits](SOURCES.md)
- [Historical plan — superseded](PLAN.md)
- [Go service development guide](apps/workflow/README.md)
- [Current S01a ticket and test evidence](plans/S01a-backend-foundation.md)

## Development status

**Go service and PostgreSQL persistence foundations work locally; the authenticated Workflow API is not ready yet.**

| Component | Current state |
|---|---|
| Go service and PostgreSQL | pgx pool, Goose SQL migrations, sqlc-generated readiness queries and project/membership schema implemented and locally proven |
| Authentication, `/pipeline/v1` projects and run/job/approval/enrollment APIs | Not implemented; overall readiness stays 503 |
| APID/AuthD | Existing code/contracts reviewed; target live environment and access not validated |
| Rust stitcher and Python capture core | Existing reusable implementations; identified hardening and integration remain |
| Headless clients/acceptance scripts, capture helper protocol, Tauri UI | Go probe client in the repo; local smoke harness kept separately; full acceptance, helper protocol and UI pending |

Application implementation has started only as limited local groundwork: the
S01a Go backend foundation on `slice/s01a-backend-foundation`. The backlog
still begins with **S00: lock the pilot scope and unblock access**; its
unresolved decisions and access block S01/S05 acceptance, not local groundwork.
The [S00 decision sheet](plans/S00-pilot-scope.md) records desktop-shell/capture
reuse, backend-first delivery, Go orchestration and pnpm workspace tooling as confirmed;
remaining scope, framework, environment and access decisions still block S00.

Use a separate branch for each slice. Keep tests, review evidence, and scope
changes with that slice; do not mark proposed or mocked behavior as validated.
Creating this repository does not yet decide where the Workflow backend will
live; that remains an S00 decision. The S01a Go foundation is developed under
`apps/workflow/` here in the meantime.

## Workspace and backend development

Use Node **24.14.0**, pnpm **11.20.0**, and Go **1.27.1** (version files and
`packageManager` pin these). Go's race detector also needs a C compiler. Unit
checks need no database, cloud credentials, camera or reference checkout.
The separate persistence proof uses an owned temporary PostgreSQL 15+ cluster.

```sh
pnpm install --frozen-lockfile --ignore-scripts
pnpm check   # sqlc diff, go vet and uncached race-enabled unit tests
pnpm db:generate  # regenerate typed Go after SQL changes
pnpm fmt     # format Go source with gofmt
pnpm build   # bin/workflow (ignored)
pnpm dev     # foreground service on 127.0.0.1:8080; Ctrl-C to stop
```

`GET /healthz` returns 200. `GET /readyz` now performs actual database/schema
checks, but overall status remains 503 `NOT_READY` until the authenticated
project API exists. A ready database is not a ready enrollment workflow.

`WORKFLOW_DATABASE_URL` optionally selects an explicit local PostgreSQL URL;
without it, no database connection is attempted. `pnpm db:migrate` explicitly
applies embedded Goose SQL migrations to that database. sqlc generates typed Go
queries over pgx, and `pnpm db:check` detects stale generated output. Startup and
health probes never migrate or seed data. Goose uses per-file transactions, not
batch-wide rollback, and does not store SQL checksums. See the
[database instructions](apps/workflow/README.md#database-and-migrations), including
the guard against silently adopting the retired prototype ledger.

Workspace members are `apps/*` and future `packages/*`; references now live
outside the repository entirely. `apps/workflow/package.json` is only a Go task wrapper. Backend
dependencies belong in `apps/workflow/go.mod` and `go.sum`. Developer tooling such
as sqlc is pinned separately in `apps/workflow/tools/go.mod`, while
future UI dependencies belong to their pnpm packages. Go also builds/runs/tests
directly without Node or pnpm; see the [service guide](apps/workflow/README.md).
The redundant Makefile, custom `scripts/` folder and smoke entry points have
been removed. Standard source-adjacent unit tests remain in the repository.

### Local-only setup checks

Per the current development policy, temporary smoke/integration harnesses,
fixture generators and scratch reports stay in the sibling folder
`../label-enrollment-app-tests/`, **outside this repository and its workspace**.
They are not committed; application builds and CI must not depend on them.
Formal in-repository harnesses will be added later when requested.

On this workstation, an optional check is:

```sh
cd ../label-enrollment-app-tests/harness
GOTOOLCHAIN=local GOWORK=off GOPROXY=off go run ./cmd/smoke -repo ../../label-enrollment-app
# With PostgreSQL binaries available (PG_BIN can specify their directory):
GOTOOLCHAIN=local GOWORK=off GOPROXY=off go run ./cmd/persistence -repo ../../label-enrollment-app
```

These local drivers build/start the actual service and use its Go client over
HTTP, without copying backend code or importing private packages. Persistence
checks own their temporary database, test migration rollback/replay and outage
recovery, then clean up. Results are local evidence, not authentication or shared
CI acceptance. The harness module is separate from the sibling `reference/`
directory, so Go test discovery does not traverse reference sources.

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

Reference checkouts now live at **`../label-enrollment-app-tests/reference/`**,
not under this repository. All 12 checkouts retained their revisions; linked
worktree pointers were repaired after the move. Their source contents were not
edited. The old `reference/` ignore rule remains as a safeguard against accidental
reintroduction. Source provenance and path conventions are in `SOURCES.md`.

Do not force-add reference checkouts, credentials, raw captures, or private
fixture data. Follow each reference repository's instructions; in particular,
do not read or change the stitcher's protected `manifests/` directory.
