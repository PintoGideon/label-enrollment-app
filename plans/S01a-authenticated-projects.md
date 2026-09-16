# S01a — List authorized projects

**Status:** implemented locally on `slice/s01a-project-list`; actual-service/client
harness proof (including real key-cache expiry), `pnpm check` and `pnpm build`
passed. No new application test files. Human/remote review and live AuthD
qualification remain pending.
**Base:** `23df567` (Go/pgx/sqlc/Goose foundation).
**Branch:** `slice/s01a-project-list`.
**Outcome:** a caller supplies an AuthD token and receives only their projects
through the real Workflow API and Go client. One endpoint, one reviewable PR.

## Contract

`GET /pipeline/v1/projects?limit=50&cursor=<last-project-uuid>`

- Verify the configured AuthD issuer, Workflow audience, signature and token
  lifetime with maintained JWT/JWKS libraries. Derive identity from verified
  `(issuer, subject)`; client headers and token org claims do not grant membership.
- Query existing `projects` / `project_memberships` using sqlc. Filter by principal
  before pagination; return each project once, ordered by UUID, with roles in
  `capture`, `process`, `review`, `enroll` order. Default limit 50, allowed 1–100;
  malformed limits/cursors return 400 after authentication.
- Return `{ "projects": [{ "id": "…", "name": "…", "roles": ["review"] }],
  "nextCursor": null }`. No membership returns an empty array. Use the last
  returned project UUID as `nextCursor` only when another page exists.
- Missing/invalid token: 401. Required database/key dependency unavailable: 503.
  Reuse the existing JSON error envelope; keep secrets and raw upstream errors
  out of responses/logs. Verify cached keys only within the bounded policy.
- Add one client method, `ListProjects(ctx, token, options)`, using the existing
  bounded HTTP client and redirect refusal. Tokens are supplied per call.

Auth verification retains the [identity decision's](authentication-boundaries.md)
trusted HTTPS JWKS, bounded cache/refresh, allowed algorithms, claim checks and
no-bypass rules. Keep the verifier small; do not build a new identity framework.

## Application change boundary

| Area | Change |
|---|---|
| `internal/config`, new `internal/auth` | Explicit issuer/audience configuration and maintained-library verification |
| `internal/database/queries`, generated `dbsql` | One membership-filtered project-list query; existing schema |
| `internal/httpapi`, `cmd/workflow` | Wire authentication and the single route into the current server |
| `pkg/api`, `pkg/client` | List/page contract and one client method |
| `go.mod`, `go.sum` | Maintained JWT/JWKS dependencies; new checks remain in the external harness |

Reuse the current pool, migration runner, server and client. `/healthz` and
`/readyz` retain their current status/contract; readiness remains 503 until the
later project-API readiness increment. If a diagnostic says authentication is
unimplemented, correct that text without expanding the readiness contract.
No migration or project-detail route is needed.

## Proof and review

All process/database/issuer harnesses, fixture generators, scripts and reports
belong in the sibling **`../label-enrollment-harness/`**, outside this repository
and pnpm workspace. Per the user's first-cut instruction, add no new test files to the application;
this slice's checks live in the external harness. Pre-existing foundation tests
remain in place, with only necessary wiring updates. The harness
consumes the public client and starts the actual service, owned PostgreSQL and
ephemeral TLS issuer. Production builds and CI never import the harness.

Prove authorized/empty lists, multi-role deduplication, pagination and project
isolation; deny missing/forged/expired/wrong-issuer/wrong-audience tokens and
spoofed context. Exercise key rotation/outage, database outage, service restart
and safe logs. Run `pnpm check` and `pnpm build`; retain detailed integration
output in the harness and put only commands, outcome and limitations in the
PR description. This is local evidence, not live AuthD or full S05 acceptance.

**Review rule:** the feature diff contains only the application changes above
and this ticket's status. Harness relocation is outside this slice and has not
been performed; keep broad planning updates separate. No copied server, generic test framework, project
scripts, migration/CI cleanup or unrelated refactoring in the feature PR.

**Follow-up:** project detail and authenticated-API readiness, then S05's remaining
context/permission qualification. Native login, run ownership, uploads, processing
and enrollment stay in their existing slices. S00/S01/S05 parent gates remain open.
