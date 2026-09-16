# Go Workflow service

Local Go Workflow: authenticated project discovery, PostgreSQL run/command/event
persistence, verified S3-compatible inputs and local execution of the existing
Rust stitcher. Processing review/approval passed the external synthetic harness;
a latest-stitcher 50-frame real-data inspection also passed, with approval blocked
for missing serial authority. This is not a deployed/scientifically qualified
or APID enrollment service.
Rust remains responsible for image processing; future Go enrollment workers call
APID directly without invoking clid.

## Layout

```text
apps/workflow/
  package.json                 pnpm task wrapper, not a JavaScript backend
  go.mod / go.sum               runtime dependencies: pgx, Goose, JWT/JWKS and AWS S3 SDK
  tools/go.mod / go.sum         developer tooling module: sqlc v1.31.1; never a service dependency
  sqlc.yaml                    SQL inputs and pgx/v5 Go generation settings
  cmd/workflow/                 serve/migrate commands and signal handling
  internal/config/              explicit local listener/database configuration
  internal/database/            pool, Goose adapter and read-only readiness
    migrations/001_projects.sql embedded Goose SQL; application schema authority
    schema/goose.sql            sqlc-only description of Goose-owned metadata
    queries/                    readiness, membership and processing SQL
    dbsql/                      checked-in generated Go; do not hand-edit
  internal/auth/                bounded Workflow JWT/JWKS verification
  internal/processing/          input snapshots, local runner, result/review control
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

The service consumes generated readiness, project membership and processing
queries. Command receipts, history and revision changes commit transactionally.
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
| `GET /pipeline/v1/projects` | Authenticated membership-filtered project page |

Database states are `not_configured`, `unavailable`, `migration_required`,
`schema_mismatch` or `ready`. Even a ready database does not make an unimplemented
application ready. The readiness marker is intentionally still deferred; it does
not mean the project-list route is absent. Example after applying migrations:

```json
{"error":{"code":"NOT_READY","message":"Authenticated project API readiness is not implemented yet."},"checks":{"database":"ready","projectApi":"not_implemented"}}
```

`pkg/client` exposes validated checks on `*client.APIError.Checks`. It caps bodies
per endpoint (64 KiB probes, 256 KiB projects, bounded larger processing data),
applies a three-second timeout, honors cancellation and refuses
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
database outage/restart, sanitized migration logs and owned resource cleanup.
That historical foundation proof used no existing DB, AWS/APID/Google service or
real label data; the later 50-frame processing inspection is separately documented.

References are separately under `../label-enrollment-app-tests/reference/`, outside
both the app and harness modules. Never recursively run Go tools from that parent
container or inspect the stitcher's protected `manifests/` directory.

[S01a project listing](../../plans/S01a-authenticated-projects.md) is committed.
[ORCH-01](../../plans/ORCH-01-processing-orchestration.md) is in progress. All new
proof scripts, fixtures and reports are in `../label-enrollment-harness/`; no new
application test files are being added for this first cut. Existing foundation
tests remain. S00/full parent, scientific, live AuthD/cloud, native/UI and release
gates are separate.

## Local processing

`WORKFLOW_PROCESSING_CONFIG` explicitly selects an administrator-owned JSON policy.
It requires authentication and a migrated database. No policy means processing
access is unavailable; there is no unauthenticated/test bypass. The policy selects
an absolute workspace and local Docker Unix socket, an immutable image ID, a
literal-loopback S3-compatible endpoint with explicit credentials, project/source
bucket-prefix allowlists and frozen profile assets. Keep its credentials private.
It never discovers cloud credentials, uses a remote Docker context or calls APID.

Under `/pipeline/v1/projects/{project}/runs`:

- `POST` registers exact sorted filenames, sizes and SHA-256s.
- `GET /{run}` and `/events` retrieve durable state/progress.
- `POST /{run}/verify`, `/start`, `/cancel` control asynchronous processing.
- `GET /{run}/results/{attempt}` and `/artifacts/{sha256}` under that result inspect
  published result metadata and PNG bytes; no executable engine HTML is served.
- `POST /{run}/review`, `/approve` bind explicit decisions to an immutable result.
- `GET /{run}/approvals/{approval}` retains historical processing-only approval.

Commands require a verified human, a UUID `commandId` and (after registration)
current `revision`. Current project roles apply: capture registers; process
verifies/starts/cancels; review reviews/approves; any member may inspect. Owner and
individual action actors remain distinct. Replays reauthorize and return their
original receipt; changed payloads/stale revisions conflict. Worker claims are
leased/fenced, recheck authority and reconcile deterministic execution identities.
A persistent launch guard prevents an ambiguous Docker start from executing the
algorithm twice; uncertainty never implies success.

The local runner invokes the pinned `stitchin-complete` binary, without the
upstream false-success logging wrapper, `--resume`, user tokens or networking.
Inputs/assets are read-only; each attempt has separate output, four workers/four
CPU quota, 4 GiB memory and a one-hour execution deadline. Host workspace/socket
must remain stable for recovery. Existing containers and output are retained;
production retention/cleanup and cloud deployment are not qualified here.

Inputs are bounded at 10,000 frames / 16 GiB total / 16 MiB each. Complete S3
inventory and downloaded bytes are checked, versions recorded, and a disk
snapshot sealed before processing; entire captures are not accumulated in RAM.
Filename case collisions are rejected. Verification is cancelable and bounded
at 15 minutes. Asset bundles retain a separate 256 MiB/1,000-file bound.

A `synthetic` profile requires explicit QR/serial reference data and expected
serials. An `inspection` profile may process a provided capture without inventing
those identities (`reference` empty, `expectedSerials: []`, direction may be
`auto`). It deliberately ends with `REFERENCE_REQUIRED` after successful engine
execution: raw outputs remain inspectable in the owned workspace, but no approved
result is published. Correct physical associations and enrollment eligibility
must not be inferred from process exit, crop counts or QR decoding alone.

Inputs use the actual S3 SDK. Workflow's current published artifact store is the
owned local workspace; harness archival of original outputs to local object
storage is not a cloud-S3 publication/recovery guarantee. Live AWS storage and
APID enrollment require separate configuration, permission and qualification.
