# Go Workflow service

Local S01a foundation: HTTP lifecycle, a real PostgreSQL connection pool and
explicit Goose migrations and sqlc-generated queries over pgx.
**Authentication and project HTTP APIs are pending.**
Rust remains responsible for image processing; future Go enrollment workers call
APID directly without invoking clid.

## Layout

```text
apps/workflow/
  package.json                 pnpm task wrapper, not a JavaScript backend
  go.mod / go.sum               service dependencies only: pgx v5.11.0, Goose v3.28.0
  tools/go.mod / go.sum         developer tooling module: sqlc v1.31.1; never a service dependency
  sqlc.yaml                    SQL inputs and pgx/v5 Go generation settings
  cmd/workflow/                 serve/migrate commands and signal handling
  internal/config/              explicit local listener/database configuration
  internal/database/            pool, Goose adapter and read-only readiness
    migrations/001_projects.sql embedded Goose SQL; application schema authority
    schema/goose.sql            sqlc-only description of Goose-owned metadata
    queries/                    SQL query sources (currently readiness)
    dbsql/                      checked-in generated Go; do not hand-edit
  internal/httpapi/             routes, resource bounds and server lifecycle
  pkg/api/                      shared Go probe/error/readiness contracts
  pkg/client/                   reusable nonvisual HTTP client
```

`internal/` packages are private to this module tree. The client and API packages
are reusable Go code, not npm packages. There is no runtime/test dependency on
external reference checkouts or the local-only harness. Future native Rust/web
client gates remain separate; operator/APID tokens will not enter the renderer.

## Run and check

From the repository root, with Node 24.14.0, pnpm 11.20.0 and Go 1.27.1:

```sh
pnpm install --frozen-lockfile --ignore-scripts
pnpm dev
pnpm check   # sqlc diff, go vet and race-enabled unit tests; no database required
pnpm db:generate  # regenerate Go after editing SQL
pnpm db:check     # fail if generated Go differs from SQL/config
pnpm fmt
pnpm build   # bin/workflow (ignored)
```

Go's race detector needs a C compiler. Without Node/pnpm:

```sh
cd apps/workflow
go -C tools tool sqlc diff -f ../sqlc.yaml
go test -race -count=1 ./...
go vet ./...
go build -o ../../bin/workflow ./cmd/workflow
go run ./cmd/workflow
```

`WORKFLOW_ADDR` defaults to `127.0.0.1:8080`. Only literal loopback addresses are
accepted in this local increment. Port zero requests an ephemeral port. Ctrl-C or
SIGTERM initiates bounded graceful shutdown. Environment files are not auto-loaded.

## SQL-first development

- **pgx** owns PostgreSQL connections and execution, not an ORM.
- **sqlc** generates typed Go from SQL. It is pinned with Go's `tool` directive in
  the separate `tools/go.mod` module, so its compiler dependencies never enter the
  service module's `go.mod`/`go.sum`, builds or vulnerability scans. Invoke it as
  `go -C tools tool sqlc <command> -f ../sqlc.yaml`; no global installation or
  unpinned `latest` command is required. Upgrade with
  `go -C tools get -tool github.com/sqlc-dev/sqlc/cmd/sqlc@<version>` followed by
  `go -C tools mod tidy`, then run `pnpm db:generate` and commit the result.
- **Goose** owns migration parsing, version tracking, transactions and locking.
  The application only wraps explicit execution and compatibility/error policy.

Edit queries under `internal/database/queries/`, then run `pnpm db:generate` and
commit the generated `dbsql/` output with its SQL. For schema changes, add a new
ordered file such as `002_runs.sql` with `-- +goose Up`; never edit an applied
migration. sqlc reads the Goose migration directory as the application schema.
`schema/goose.sql` describes the library-owned version table only for codegen;
it is never executed by the application. Check it when upgrading Goose.

The service already consumes generated readiness queries. Authorized project
queries will follow in the auth/project slice; no unused CRUD API is introduced.
`pnpm check` (including CI) runs `sqlc diff` to detect stale generated code. That
check needs no database and is **not** a migration checksum or live drift audit.

## Database and migrations

Use only a disposable local development database you own. The application does
not provision database servers, create users, seed projects or infer credentials.
Temporary PostgreSQL provisioning lives in the separate local test folder.

`WORKFLOW_DATABASE_URL` must be a complete URL with username, password, literal
loopback IP, port, database and explicit `sslmode` (`disable`, `require` or
`verify-full`). Example **shape**, not working credentials:

```text
postgres://USER:PASSWORD@127.0.0.1:PORT/DATABASE?sslmode=disable
```

No URL means no database adapter/connection. Generic `DATABASE_URL`/`PGHOST` are
not substitutes for this setting. Remote hosts, DNS names, missing fields and
extra query overrides are rejected. Cloud database/TLS policy remains a later
deployment decision. Do not commit URLs/passwords or put them in package scripts.

After setting that variable for the intended local database:

```sh
pnpm db:migrate     # equivalent: go run ./cmd/workflow migrate
pnpm dev
```

`workflow migrate` is an application administration command, not a test harness.
Normal startup and HTTP probes **never** run it automatically. It creates:

- `public.workflow_goose_db_version`: Goose-owned version/applied state/time,
  including its initial version-zero row.
- `workflow.projects`: UUID, display name and creation time.
- `workflow.project_memberships`: issuer + subject + project + role, with a
  project foreign key and capture/process/review/enroll role constraint.

Goose applies each migration and its receipt in **one transaction per file**,
serialized by its PostgreSQL session advisory locker. Its version table/zero row
are initialized separately and may remain after a failed first migration. Earlier
successful files remain applied if a later file fails; there is no batch-wide
rollback promise. Lost commit acknowledgement is reported as uncertain, not as
success or proof of rollback. Inspect database status before retrying.

The app's read-only compatibility check rejects unknown, missing, duplicate or
unapplied version rows and missing required columns. It uses generated SQL rather
than Goose's status APIs, which can initialize metadata. Existing conflicting
application tables are not silently adopted. There are no seed rows, default
users, APID destinations or exposed down/reset commands.

**Goose does not store SQL checksums.** Edited applied files are not automatically
detected. Immutable migration review is required; checks here are not a complete
constraint/index/type/permission drift audit. SQL is embedded in the executable,
and `.gitattributes` pins it to LF for reproducible generation/builds.

The retired prototype's `workflow.schema_migrations` ledger is deliberately
rejected without conversion or deletion. Use a fresh owned disposable database
for local development; any retained database needing adoption requires a separate
reviewed transition. Never drop or reset an existing/shared database to bypass
this guard.

Recovery on a **disposable** local database: a dropped `workflow` schema with an
intact Goose ledger reads `schema_mismatch`, and `workflow migrate` refuses to
run. Reset such a database by also dropping `public.workflow_goose_db_version`;
readiness then reads `migration_required` and `workflow migrate` recreates the
schema. Never apply this to a shared database.

The application pool is capped at five connections; Goose uses a separate pgx
`database/sql` handle capped at one connection with no idle sessions. Construction
opens neither a connection nor a version table. Connection/readiness budgets are
two seconds, SQL has statement/lock timeouts, and migration execution has a
30-second context plus bounded locker cleanup. Dependency logs are suppressed;
only sanitized migration outcomes reach application logs. Two known limitations
are tracked in the S01a sheet: a failed migration statement currently reports no
SQLSTATE, and the migration session inherits the pool's five-second statement
timeout, which long backfills or index builds will exceed.

## HTTP and client contracts

| Route | Current result |
|---|---|
| `GET /healthz` | 200: process liveness only |
| `GET /readyz` | 503 `NOT_READY`, with actual database status and project API `not_implemented` |
| `GET /pipeline/v1/projects` | 404: not implemented; no fake project list |

Database states are `not_configured`, `unavailable`, `migration_required`,
`schema_mismatch` or `ready`. Even a ready database does not make an unimplemented
project/auth API ready. Example after applying migrations:

```json
{"error":{"code":"NOT_READY","message":"Authenticated project API is not implemented yet."},"checks":{"database":"ready","projectApi":"not_implemented"}}
```

`pkg/client` exposes validated checks on `*client.APIError.Checks`. It caps bodies
at 64 KiB, applies a three-second timeout, honors cancellation and refuses
redirects. HTTP is loopback-only; other origins require HTTPS. Untrusted response
messages/bodies do not become client error text.

## Test boundaries and next work

Ordinary unit tests remain with config, migrations, handlers and client source.
Client tests use explicit `httptest` response fixtures; `pnpm check` needs no DB.
GitHub CI checks generated SQL code and builds/tests the application without any
local-only folder dependency. Remote CI execution is not yet validated.

Existing integration code remains in `../label-enrollment-app-tests/harness/`;
the requested destination is `../label-enrollment-harness/` relative to the
repository root. Relocation is separate and has not occurred. Its Go module consumes public API/client code using a
local replacement. It builds the actual service and owns an isolated PostgreSQL
cluster. Proven locally: sqlc-backed read-only readiness, no startup migration,
retired-ledger rejection, per-file rollback on DDL collision, concurrent/repeated
migration, role/FK constraints, service restart, version/column mismatch rejection,
database outage/restart, sanitized migration logs and owned resource cleanup. No existing
DB, AWS/APID/Google service or real label data was used.

References are separately under `../label-enrollment-app-tests/reference/`, outside
both the app and harness modules. Never recursively run Go tools from that parent
container or inspect the stitcher's protected `manifests/` directory.

Next: [S01a project listing](../../plans/S01a-authenticated-projects.md)
after foundation `23df567`: AuthD JWT/JWKS verification for the configured Workflow
audience, one membership-filtered project-list route, one Go client method and
proof in `../label-enrollment-harness/` relative to the repository root. Detail
and readiness changes follow separately. Auth code
is not implemented yet. The [identity decision](../../plans/authentication-boundaries.md)
defines single login and human accountability; run/approval ownership schema comes
later. S06 run registry follows S05 acceptance. S00/full S01/S05, live service,
scientific and UI gates remain separate. All integration harnesses remain outside
the project; focused source-adjacent unit/contract tests stay with the code.
