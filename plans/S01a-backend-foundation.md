# S01a — Local backend foundation and scripted API proof

**Branch:** `slice/s01a-backend-foundation` (based on the S00 planning branch)
**Status:** Go/pnpm service, pgx pool, Goose SQL migrations and sqlc-generated
readiness queries implemented and locally proven. Signed
authentication/project APIs are next. No full parent or pilot gate is complete;
human review is pending.

## Scope and authorization

The user asked to start building after selecting backend -> API -> nonvisual
client -> scripted proof -> UI, then selected **Go for orchestration** and a
**pnpm workspace**. The earlier uncommitted TypeScript backend manifests and
dependencies have been retired; React/TypeScript/Vite remains only a frontend
proposal. This ticket covers limited local groundwork from S01/S05: Go,
PostgreSQL and a read-only authenticated project API in this repository.

pnpm manages repository tasks and future web packages. The service and headless
client are Go; `apps/workflow/package.json` only invokes Go commands. Go owns its
dependencies through `go.mod`/`go.sum`. The repository uses standard Go tools;
custom test scripts and drivers are not part of this checkpoint's source tree.

The user explicitly selected **pgx + sqlc + Goose** for SQL-first persistence.
Goose replaces the prototype's custom runner/ledger; sqlc generates typed Go
from SQL and is pinned as a Go tool in the separate `apps/workflow/tools/go.mod`
module, so the service `go.mod` carries only runtime dependencies. Generated
source stays checked in and `pnpm check` verifies it with `sqlc diff`. No
TypeScript backend or ORM is added.

S00's unresolved pilot/access decisions remain unresolved. Local build kickoff
is not approval of compute, final deployment/home/owners, label profiles,
serial/position rules or live writes. This ticket does not waive full S01/S05
acceptance or implement enrollment, upload, camera access, Tauri or UI.
Reference source stays unmodified at `../label-enrollment-app-tests/reference/`,
outside both application and harness modules. Only Git move metadata was repaired.

## Current harness-placement policy

The user requested temporary setup tests in a separate, uncommitted folder and
cleanup of the project. Drivers/tests live under the sibling
`../label-enrollment-app-tests/harness/`, which has its own local-only Go module. It builds
and launches the actual application and consumes public Go API/client packages,
without copying the backend or bypassing its `internal/` boundary.

The JS formatting checker and redundant Makefile were removed from the repo and
preserved under `../label-enrollment-app-tests/_archive/`. Project `scripts/`, `cmd/smoke`,
`internal/smoke`, `pnpm smoke` and `pnpm fmt:check` are gone. Ordinary Go unit tests,
standard pnpm/Go commands and build/unit-test CI remain. No production module,
pnpm command or CI step depends on the sibling folder. Future temporary database,
auth and integration harnesses go there; formal committed harnesses are deferred
until requested. Local results are not automatically shared CI/release acceptance.

## Reuse and boundaries

- Use `/pipeline/v1` for the planned project API and preserve project isolation.
- Use Go's `net/http`, `context`, `encoding/json`, `slog` and testing tools. Add
  maintained PostgreSQL and OIDC/JWT libraries when needed; do not invent auth.
- PostgreSQL access is pgx v5. Migrations are Goose v3 SQL files applied one
  transaction per file under a PostgreSQL session advisory lock. Typed queries
  come from sqlc v1.31.1, invoked as `go -C tools tool sqlc <command> -f ../sqlc.yaml`
  from `apps/workflow/`, so tooling never enters the service dependency graph.
- Future token verification must check issuer, audience, signature and lifetime;
  authorization uses issuer + subject membership, not email or client Team headers.
- Future isolated auth fixtures use local signing keys with actual cryptographic
  verification, never a bypass accepted by normal production configuration.
- Full acceptance must own its temporary PostgreSQL cluster, keys and service;
  no existing DB, AWS/APID/Google calls or silent in-memory database substitute.
- The external local probe smoke test is only a foundation checkpoint, not full
  acceptance. `/readyz` deliberately returns 503 and project routes 404.
- The Go client does not pass S10a's future native/web frontend-client gate.

## Implemented increment — PostgreSQL persistence (local proof, review pending)

- [x] Add the maintained pgx v5.11.0 driver, explicit local-only database configuration
      and a bounded connection pool; never log a DSN or inherit a default database.
- [x] Replace the prototype runner with embedded Goose SQL migrations for project
      and membership tables. `workflow migrate` remains an explicit operational
      command, not startup migration or a test harness. Goose owns locking,
      transactions and its version ledger; transactions are per file, not per batch.
- [x] Add sqlc-generated read-only readiness queries over pgx, using the migration
      directory as application schema input, plus generation/diff task wrappers.
      The sqlc-only Goose table description is never executed by the application.
- [x] Replace the database placeholder with real connectivity/schema checks in
      `/readyz`, exposed through the existing Go client. Overall readiness remains
      503 while the authenticated project API is unimplemented.
- [x] Unit-test configuration, migration-history checks and HTTP/client contracts.
      Prove actual migrations, repeat/concurrent apply, restart, schema mismatch
      and database failure using an owned temporary PostgreSQL cluster outside
      the repository. Do not touch an existing database or real label data.

Reference checkouts have moved to `../label-enrollment-app-tests/reference/`;
only Git worktree pointers were repaired, with all checkout HEADs preserved.
The local Go test module is now under `../label-enrollment-app-tests/harness/`
so normal Go discovery cannot traverse reference sources or protected manifests.
The app still has no dependency on either external folder.

## B — Backend

- [x] Create a pnpm workspace task boundary and a standalone Go module.
- [x] Implement loopback configuration, HTTP resource bounds, liveness,
      fail-closed readiness, structured startup logs and graceful shutdown.
- [x] Implement the pgx pool, project/membership schema, Goose versioned SQL
      migrations and sqlc-generated dependency/schema-backed readiness.
- [ ] Surface the PostgreSQL SQLSTATE and message for a failed migration
      statement (Goose logs are currently discarded) while keeping connection
      errors sanitized.
- [ ] Lift the pool's five-second `statement_timeout` for the migration session
      so backfills and large index builds can run.
- [ ] Implement authorized project/membership queries with signed identities.
- [ ] Implement signed-token verification and authenticated/request-aware logging.

## A — API

- [x] Add executable Go probe/error contracts. Do not invent project responses.
- [ ] Add language-neutral project/pagination/error contracts and authenticated
      `GET /pipeline/v1/projects` plus authorized project detail.
- [ ] Enforce server-owned membership, stable errors and request IDs;
      unknown/inaccessible projects must not leak existence.

## C — Nonvisual client

- [x] Add the Go probe client: response validation, bounded reads, cancellation,
      timeouts, redirect refusal and no untrusted body text in errors.
- [ ] Add authenticated project methods and the broader contract/failure tests.

## T — Scripted proof

- [x] Test configuration, actual HTTP probe handlers/lifecycle and Go-client errors.
- [x] Move the smoke harness and temporary tooling out of the project; run the
      external driver against the actual compiled service/public client. Canceled
      proof fails without passing evidence. No database is involved yet.
- [x] Keep local run instructions and pinned Go/Node/pnpm generation/build/unit-test
      CI, independent of scratch files. GitHub-hosted execution is not yet validated.
- [ ] In the separate local folder, exercise actual service + isolated PostgreSQL
      + signed-token issuer; prove migrations, pagination, membership, auth denial,
      restart durability, readiness failures and redacted client/API evidence.
      Do not introduce project harness code until the user requests it.
- [ ] Add the CI rule that fails a pull request which modifies or deletes a
      migration file already present on `main`. This replaces the retired
      in-database checksum ledger as the way the checksum requirement is met.
- [ ] Add `gofmt -l` enforcement to CI; formatting is currently unenforced.
- [ ] Add a PostgreSQL service job in CI that runs `workflow migrate` and checks
      that `/readyz` reports `database: ready`, so the database proof is
      reproducible rather than local-only.

## U — UI

Not part of this ticket. Passing headless proof is a prerequisite, not UI or
production-readiness acceptance.

## Evidence — 2026-09-15 EDT

Passed locally on macOS arm64, Go 1.27.1, Node 24.14.0 and pnpm 11.20.0:

- `pnpm install --offline --frozen-lockfile --ignore-scripts` (no JS dependencies).
- After cleanup, `GOTOOLCHAIN=local pnpm check`: `go vet` and uncached race-enabled
  source-adjacent Go unit tests; the SQL-first switch adds `sqlc diff` below.
  No custom scripts or external harness dependency.
- `GOTOOLCHAIN=local pnpm build`: compiled `bin/workflow` (ignored by Git).
- From `../label-enrollment-app-tests/harness/`, with `GOTOOLCHAIN=local GOWORK=off GOPROXY=off`:
  `go vet ./...`, `go test -race -count=1 ./...` and
  `go run ./cmd/smoke -repo ../../label-enrollment-app` passed. The moved harness now
  checks formatting, actual process startup, typed HTTP, fail-closed readiness,
  cancellation and graceful process shutdown.
- Historical pre-cleanup checks also passed formatting-failure propagation and
  compiled-binary 200/503/404, log-redaction, SIGTERM and public-listener rejection
  scenarios. Their old `pnpm smoke` / `pnpm fmt:check` entry points were retired.
- pnpm discovers only the root and Workflow wrapper. Reference files remain
  ignored/untracked; the test folder is outside the app's Git/workspace scope.

- Before the SQL-first replacement, the separate `go run ./cmd/persistence
  -repo ../../label-enrollment-app -pg-bin /opt/homebrew/opt/postgresql@15/bin`
  passed against an owned temporary PostgreSQL 15.19 cluster: actual service/client,
  no startup migration, atomic rollback, concurrent/repeated apply, membership
  constraints, service restart, checksum mismatch denial, database outage/restart
  and sanitized migration logs. Its checksum result belongs to the retired custom
  runner, not Goose. Current proof is recorded below.
- All 12 moved checkout HEADs and worktree top-level paths were verified using
  Git metadata only. No protected manifest or reference source was inspected.

The no-database smoke is a narrow HTTP check; the persistence proof is separate.
Neither proves authenticated project access, full S01a/S05 acceptance, deployment,
real image associations or enrollment. These remain explicit next steps. No
existing database, camera/stitcher behavior or live application service was changed.

### SQL-first replacement — local proof, human review pending

- Pinned pgx v5.11.0 and Goose v3.28.0 in the service `go.mod`. sqlc v1.31.1 is
  pinned in the separate `apps/workflow/tools/go.mod`, so the service module's
  `go.mod`/`go.sum` carry no compiler dependencies and the service never executes
  or deploys sqlc.
- `GOTOOLCHAIN=local pnpm check` passed sqlc diff, vet and uncached race tests;
  `pnpm db:generate`, `pnpm build`, frozen offline pnpm install and `go mod verify`
  for both service and tools modules also passed. The no-DB smoke passed again.
- A temporary-only sqlc probe reproduced the generated production SQL output and
  verified nonzero exits for stale generated Go and an invalid SQL column.
  No application files were mutated for these negative cases.
- The updated external persistence driver passed on an owned PostgreSQL 15.19
  cluster through the actual executable and public Go client: generated read-only
  readiness, no startup/metadata initialization, retired-ledger refusal, rollback
  of earlier statements in a failed migration, concurrent/repeated apply,
  role/FK constraints, service restart, unknown/duplicate/unapplied/missing-zero
  history and missing-column rejection, database outage/restart and sanitized logs.
  Passing evidence was emitted only after owned process/cluster/temp cleanup.
- Goose uses `public.workflow_goose_db_version`, including its zero row. That
  metadata is initialized separately; each SQL file and its receipt are one
  transaction. Earlier successful files can remain if later files fail. Errors
  never promise batch rollback or assume a lost commit acknowledgement failed.
- **Goose does not store SQL checksums.** No replacement checksum engine was
  added. Applied files must remain immutable in review; sqlc diff checks generated
  code consistency, not historical SQL integrity or full database drift.
- Databases containing the retired `workflow.schema_migrations` table fail closed.
  No silent conversion, data reset, seed or down command was added. Any retained
  database transition requires separate review; this work used only fresh fixtures.
- **2026-09-15 17:43 EDT, planning-session driver against the Goose runner:** the
  compiled `workflow` binary ran against an owned temporary PostgreSQL 15.19
  cluster with Unix sockets disabled. Migrate on an empty database applied 1 and
  a replay applied 0; `/readyz` reported `database: ready`. A future Goose version
  row, the retired `workflow.schema_migrations` table, a dropped required table
  and a dropped `workflow` schema each produced `schema_mismatch`, and `workflow
  migrate` refused to run in every case. Dropping the Goose ledger produced
  `migration_required` and migrate restored the schema. A database outage produced
  `unavailable` and recovered on restart. SIGTERM exited 0. Six malformed URLs and
  a wrong password failed without echoing the password. Cluster and binary were
  deleted afterwards. The driver is uncommitted scratch, not CI evidence.
- **2026-09-15 EDT, tooling split:** sqlc moved from the service `go.mod` into
  `apps/workflow/tools/go.mod`. The service module now requires only pgx and
  Goose plus eight indirect modules; `go mod tidy -diff` is clean in both modules;
  `go -C tools tool sqlc diff -f ../sqlc.yaml` reports no drift; `go list ./...`
  in the service module does not include `tools/`; `pnpm check` and `pnpm build`
  pass through the updated `db:check`/`db:generate` scripts.

This changes persistence tooling only. Authentication/project APIs, S00/full
S01/S05 acceptance, native/UI integration and live enrollment remain pending.
