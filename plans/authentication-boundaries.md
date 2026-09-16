# AuthD authentication, workflow ownership and end-to-end data flow

**Status:** approved identity/ownership decision. Its bounded
[S01a project-list increment](S01a-authenticated-projects.md) was implemented at
`31c589f`, following foundation `23df567`. Local processing, ownership and
processing-result approval now live in [ORCH-01](ORCH-01-processing-orchestration.md).
The original target sequences below also include later native/cloud/APID work;
they are not permission to expand this increment or evidence of live qualification.
The current execution request is commit/push onto the previous slice branch,
then test the 50-frame orchestration path before starting Tauri. Native UI,
live AuthD/cloud and APID enrollment remain unimplemented/unqualified.

## Context

The user clarified the product goal: **Labeltron controls the end-to-end workflow
through a cloud Workflow orchestrator, which invokes the cloud stitcher algorithm
and manages approved APID enrollment.** Both orchestration and stitching run in
the cloud; the Labeltron desktop remains the operator's entry point. One click
starts processing and the app displays durable end-to-end progress.

**Deployment is deferred. A Kubernetes Job is not a requirement.** Do not couple
the application/authentication design to Kubernetes or fold orchestration/APID
enrollment into the image-processing algorithm. Operators do not manage compute
or separately log into workers. Review/approval and enrollment remain part of
the same application experience.

**Confirmed responsibility model:** the signed-in person is accountable for the
end-to-end workflow. Persist their server-verified AuthD `(issuer, subject)` as
the responsible principal; never persist a bearer token as ownership evidence.
The AuthD service account is the background executor, not a substitute human or
an ownership transfer. This is one application authentication system with
separate user and workload identities.

**Scope of this decision:** record the agreed authentication/ownership architecture
and define the bounded S01a resource-server/project-discovery increment. The complete
data flow is the target for subsequent slices, not permission to implement the
whole pipeline at once. Live deployment, platform access, native/UI builds and
real extraction/enrollment remain separately gated.

One-click start means launching a processing run, not provisioning infrastructure.
Cloud placement for Workflow and the stitcher is confirmed; the hosting platform,
invocation transport, packaging and deployment topology remain later decisions.
The current foundation still runs locally and neither component has been deployed
in this work. The earlier plan review closed without an approval or rejection.

### Existing stitcher evidence — not a deployment selection

Reference root: `../label-enrollment-app-tests/reference/labeltron-two-stitcher/`.
Protected `manifests/` remains uninspected; no cluster or cloud commands were run.

- The existing Rust engine and its input/output/profile contracts are the reuse
  target. `docker/entrypoint.sh::s3_run` demonstrates transfers around that engine.
- The reviewed `k8s/job.yaml`, `k8s/serviceaccount.yaml`, `k8s/run-job.sh` and
  `k8s/README.md` describe one deployment example, **not a required architecture**.
  No deployed compute, IAM grants or project isolation were verified.
- Workload invocation, storage access and APID enrollment are distinct permission
  boundaries regardless of the eventual hosting platform. None requires putting
  the operator's token or an APID credential inside the stitcher.

### AuthD/APID evidence for a unified login

The user confirmed **one AuthD login to control Workflow and approved enrollment
in an org/Team**, with the signed-in human responsible end to end.
Source review supports one identity provider, but must not be confused with every
API accepting every token:

- AuthD `src/lib/auth.ts`: human authorization-code/refresh-token OAuth flows;
  issuer `${APP_URL}/api/auth`; registered API audiences. The separate JWT-plugin
  configuration uses the allowed audience list. These are distinct issuance paths.
- AuthD `src/lib/dust-plugin/service-accounts.ts::serviceAccountToken`: service
  credentials issue an AuthD JWT with one explicitly allowed `resource` audience,
  defaulting to APID. Do not assume this endpoint mints a multi-audience token.
- APID `apps/apid/src/lib/authd-jwt.ts::verifyAuthDJwt`: checks AuthD signature,
  issuer, accepted audience, lifetime and DUST claims.
- APID `apps/apid/src/lib/ops/composite-tags/_helpers.ts`:
  `checkCompositeTagEnrollmentAdmin` permits either a human with Team/org admin
  authority or a service principal belonging to the selected Team. **An AuthD
  service account is not mandatory for every enrollment call.** It is the
  recommended executor for durable unattended enrollment, not the only valid actor.
- APID checks the selected Team belongs to the selected org and queries actual
  membership. Context headers select a destination; they do not grant access.
- APID `routes/v1.ts` exposes `/api/v1/me` from verified claims; `routes/v1/groups.ts`
  exposes `/api/v1/teams`. The latter can include linked/readable Teams, so listing
  a Team is not proof of enrollment permission. The current-permission lookup
  contract must be qualified before a more privileged worker can act for a user.
- The locally installed `@better-auth/oauth-provider` v1.6.9 token endpoint accepts
  one `resource` string. Its helper can form audience arrays, but that does not
  establish arbitrary multi-resource issuance through the public token endpoint.
  Do not promise that one native-login JWT already covers Workflow and APID.

These are reviewed source contracts, not a live AuthD/APID integration proof.

## Approach

### One application controls the whole workflow

```text
                         AUTHD: ONE APPLICATION IDENTITY PROVIDER
                    +------------------------------------------------+
                    | Human users / organizations / Teams            |
                    | OAuth authorization code + PKCE / token renewal|
                    | Service accounts / service-token exchange      |
                    | Public JWKS for token signature verification   |
                    +------------------+--------------------+--------+
                                       |                    |
                              user login/token       service credential
                                       |             exchange -> APID token
                                       v                    |
+---------------- LABELTRON STATION ----------------+       |
|                                                  |       |
| System browser <---- AuthD login                  |       |
|       | approved callback + authorization code    |       |
|       v                                          |       |
| Tauri native Rust                                |       |
|   - checks state/PKCE; exchanges code             |       |
|   - access token in memory; refresh in OS vault   |       |
|   - owns authenticated HTTPS and progress stream |       |
|       ^                            |             |       |
|       | scoped commands/events     | capture IPC |       |
|       v                            v             |       |
| Bundled web UI                Python capture     |       |
| No bearer/refresh tokens      No cloud tokens    |       |
|                              -> sealed raw files|       |
+---------------------+----------------------------+       |
                      | HTTPS: Bearer USER token           |
                      | aud = Workflow                     |
                      v                                    |
+-------------------- CLOUD GO WORKFLOW BACKEND -------------------+
| AuthN: trusted issuer + signature + audience + token lifetime      |
| AuthZ: project/action permission + validated org/Team/object scope |
| State: input readiness / frozen approval / cancellation guards    |
|                                                                  |
| PostgreSQL                                                       |
|   responsible human + action actors + run state + approved intent |
|   processing attempts + progress events + enrollment receipts     |
|                                                                  |
| Upload signer       Processing control        Enrollment worker  |
|   |                 and progress              |                  |
|   |                 |                         |<-- AuthD token --+
|   |                 |                         |    aud = APID
+---+-----------------+-------------------------+------------------+
    | scoped URLs     | private invocation      | Reads approved crops
    | for native app  | transport TBD           | APID bearer service token
    |                 v                         | validated org/Team context
    |           +----------------------+        v
    |           | Cloud stitcher       |   +-------------------------+
    |           | Rust algorithm       |   | APID                    |
    |           | No operator tokens   |   | Verifies AuthD token    |
    |           | No APID credentials  |   | Checks Team authority   |
    |           +----------+-----------+   | Extracts / enrolls      |
    |                      |               | Returns IDs / receipts  |
    |                      | scoped S3     +------------+------------+
    |                      | access                     |
    v                      v                            |
+------------------------------+                        |
| S3: raw inputs / assets      |                        |
| immutable results / crops    |                        |
+------------------------------+                        |
             |                                          |
             +-- result validation/import --> Workflow <-+
                                               |
                  authorized progress/results  |
                  Native Rust <----------------+
                      |
                      v
                   Web UI
```

The worker's service credential is held in a cloud secret manager, not on the
station or in PostgreSQL work payloads. Processing control and enrollment are
roles of the cloud orchestrator, not a requirement for separate deployments.
Cloud workload authentication is infrastructure plumbing, not another operator
login. Its mechanism will be selected with the hosting platform; it must preserve
scoped invocation/storage access without forwarding the human's token.

The logical data flow is **Labeltron app → cloud Workflow → cloud stitcher →
APID**. Workflow receives the stitcher's results and controls validation, approval
and APID calls; the algorithm itself does not become the orchestrator/enroller.

### Sign-in sequence

```text
Operator       Web UI       Native Rust       Browser/AuthD       Workflow
   |              |              |                  |                |
   |-- Sign in -->|-- command -->|                  |                |
   |              |              |-- authorize ---->|                |
   |              |              |  state + PKCE    |                |
   |<---------------------------- login/consent ----|                |
   |----------------------------- authenticate --->|                |
   |              |              |<-- auth code ----|                |
   |              |              |-- code+verifier->|                |
   |              |              |<-- tokens -------|                |
   |              |              |                  |                |
   |              |              |-- USER JWT ---------------------->|
   |              |              |                  |    verify JWT |
   |              |              |                  |    authorize  |
   |              |              |<-- allowed project/context -------|
   |              |<-- safe view-|                  |                |
   |<-- ready ----|              |                  |                |
```

Workflow fetches public signing keys from the configured AuthD JWKS endpoint
through a bounded cache; it does not send user tokens to arbitrary URLs or require
a password exchange on each request. `Workflow` and `APID` audience labels in the
diagram represent explicitly registered/configured resource values.

Native login/client/audience registration is a later integration gate. The first
backend slice verifies real signed fixtures without pretending native login or
live AuthD connectivity already exists.

### Command authorization and worker handoff

```text
Authenticated command: start / approve / enroll / cancel
                         |
                         v
             Verify user token against trusted AuthD JWKS
               | invalid/expired -> 401
               | dependency unavailable -> fail closed
                         |
                         v
             Derive principal from verified issuer + subject
             Ignore submitted owner/actor identities for authority
                         |
                         v
             Check org/Team/project/action/object permissions
               | denied -> 403, or non-disclosing 404 where applicable
                         |
                         v
             Check workflow state and immutable approval where required
                         |
                         v
             Persist intent + actor + context + request ID
                         |
                  durable work reference
                  (NO USER BEARER TOKEN)
                         |
             +-----------+------------------+
             |                              |
             v                              v
       Processing adapter             Enrollment worker
       workload credentials           AuthD SERVICE token -> APID
             |                              |
             +-- events/results/receipts ----+
                         |
                         v
             Durable Workflow state -> authorized client -> UI
```

Ownership is not a blanket permission grant. The applicable state/authorization
checks still gate each action, and worker cancellation/revocation policy must be
qualified before the write path is enabled.

### Accountable owner versus executing identity

```text
Run
+-- responsible_principal = verified AuthD issuer + human subject
+-- validated org / Team / project
+-- initiated_by          = verified actor for registration/start
+-- approvals
|   +-- approved_by       = verified human actor
|   +-- approval_digest  = exact artifacts + positions + destination + indexing
+-- enrollment_intent
|   +-- requested_by     = verified human actor
|   +-- approval_ref     = immutable approved plan
+-- execution_attempts
|   +-- executed_by      = AuthD service principal (enrollment)
|   +-- workload/job identity (processing)
|   +-- APID receipts / reconciliation outcome
+-- NO stored user bearer or refresh token
```

These are target record responsibilities, not claims that the run/approval schema
already exists. Token expiry/renewal does not rewrite the responsible principal.

- At cloud run creation/registration, derive the responsible principal from the
  verified human session, not a submitted `ownerId`, email or actor header.
  An offline capture has no authenticated cloud owner until this registration.
- Persist `responsible_principal`, `initiated_by`, `approved_by` and
  `enrollment_requested_by` as verified identity references where those actions
  occur. They may identify the same person; never overwrite earlier attribution
  if another authorized person performs a later action.
- Record the executing service principal separately on worker attempts/receipts.
  APID authenticates that service account; human attribution comes from the
  verified, persisted Workflow approval/intent. A declared-actor header alone
  is not verified human identity and never grants privileges.
- Ownership is accountability, not an authorization bypass. Each new command
  requires a valid token and applicable permissions; worker execution requires
  valid approved intent, permitted destination and cancellation/revocation checks.
- Closing the app, token renewal or worker restart does not change ownership.
  No implicit ownership transfer is allowed; an explicit audited transfer feature
  is deferred. Raw user access/refresh tokens never become run/job/audit payloads.

### End-to-end data flow — target architecture

0. **One-time administrator setup.** Register Labeltron's public PKCE client and
   Workflow audience in AuthD. Associate projects with permitted org/Team context,
   storage prefixes, approved profiles and enrollment destinations. Provision the
   cloud workload permissions and Team-scoped enrollment service credential.
   These are not credentials the operator enters for every run.
1. **Sign in once.** Native Rust opens AuthD in the system browser, checks the
   callback/state/PKCE flow, and holds the resulting Workflow access token and
   refresh credential securely. The renderer and Python helper receive no tokens.
   Go validates issuer, signature, audience and lifetime on API calls. No new user
   directory, password database or second Google login is introduced.
2. **Choose authorized context.** Labeltron displays allowed org/Team/project
   choices. Workflow validates the selection against current authority and its
   project/action policy; arbitrary client headers or an org claim alone do not
   grant Team access. Store the validated context on the run and reauthorize every
   subsequent action. The supported membership/permission lookup remains to be
   confirmed, not an invented already-working API.
3. **Capture/import and verified upload.** Python captures to local files; native
   Rust journals uploads after sealing. Authorized Workflow requests obtain scoped
   S3 upload URLs. File bytes go directly to S3, not through AuthD or an AuthD token
   passed to S3. Workflow verifies exact object identity/checksums/versions before
   making input eligible for processing. Existing-S3 import enters at this point
   after source-scope/identity checks. Offline capture remains possible; remote
   actions require a valid session when connectivity returns.
4. **One-click processing.** Labeltron sends an authorized start command with run
   and approved profile identifiers. Cloud Go Workflow validates permissions and
   input readiness, persists durable intent and invokes the cloud stitcher through
   a processing adapter. Invocation transport/deployment is deferred; only approved
   inputs/profiles are accepted, not arbitrary commands or desktop-supplied cloud
   credentials. Duplicate/uncertain starts are reconciled.
5. **Algorithm execution and progress.** The cloud stitcher reads approved raw
   files/assets from S3 through scoped workload access, runs the existing Rust
   algorithm and writes outputs/logs to attempt-specific storage. It receives
   neither the user's AuthD token nor the APID service credential. Workflow collects
   execution lifecycle and structured engine stage/count events into durable run
   state. The authorized native client polls/subscribes and renders progress;
   reopening the app reattaches to the same run. No invented percentage from
   infrastructure status or false success from process exit alone. Existing
   runner/progress hardening remains separate prerequisite work.
6. **Results and approval.** Workflow validates/imports exact output artifacts and
   serves authorized previews using scoped URLs. The operator reviews labels and
   explicitly approves a frozen plan: artifact versions/digests, identities,
   positions, org/Team/destination UUIDs and indexing. Record the verified approving
   subject. Reprocessing or changing the destination invalidates stale approval.
7. **Enrollment.** The operator's Workflow token authorizes the enrollment command;
   the API rechecks permissions, context and approval and persists enrollment
   intent. The background worker retrieves its Team-scoped
   AuthD service credential from a secret manager, exchanges it for an APID-audience
   token and re-exchanges as needed. It reads approved crop bytes using IAM, calls
   APID extraction and Label/Reel enrollment with explicit org/Team context, and
   checkpoints receipts. APID independently checks the service account's authority.
   Never allow the backend credential to become an unauthorized user's privilege
   escalation path. APID receives crop bytes, not a manifest or S3 credentials.
8. **Reconcile and report.** Workflow tracks per-row success/failure/uncertainty,
   reconciles ambiguous APID outcomes instead of blind replay, and returns durable
   progress/results to Labeltron. APID remains authoritative for actual enrollment.
   Audit links initiator, approver, executing service principal and exact artifacts.
   Approved background work can continue after the UI closes or its session
   expires; the app clears credentials on sign-out, but logout is not silently
   treated as job cancellation. Do not claim instantaneous JWT revocation.

Processing starts do not auto-approve or auto-enroll. The run registry, scheduler,
verified storage, result importer, approval/enrollment and native client slices
are still needed: a login/project endpoint alone does not deliver this workflow.

### Authentication behind that experience

1. **Labeltron → Workflow:** one AuthD browser/PKCE login identifies the user for
   the application. No independent Google login or separate Workflow account is
   required by this recommended target design. Native Rust owns access/refresh
   tokens; the renderer and capture helper do not. Go verifies AuthD tokens for
   Workflow and applies project/action permissions within validated org/Team
   context. Reuse existing identity/membership authorities rather than creating
   another user directory.
2. **Cloud Workflow → cloud stitcher:** private, authorized workload invocation
   through the selected processing adapter. The hosting/transport/credential
   mechanism is deferred; arbitrary execution and forwarded user tokens are not
   permitted. No Kubernetes Job or per-run infrastructure provisioning is assumed.
3. **Cloud stitcher → S3:** scoped workload access to approved inputs/assets and
   attempt-specific outputs. No human token or APID enrollment credential.
4. **Enrollment worker → APID:** use a Team-scoped service account issued by the
   **same AuthD**, not a second identity provider. Keep its credential server-side
   and audit the responsible/initiating/approving human separately from the
   executing service principal. Never elevate a caller merely because the backend
   has a more powerful credential. Direct human-token APID enrollment exists in
   the source but is not the selected durable-background execution model.

Only the first boundary needs a user-facing login. The others are
configured once by an administrator/platform owner and used automatically.
The APID service account does not, by itself, protect the Workflow control API.

### One login versus one literal JWT

Use explicit audience-specific tokens from the same identity provider:

- The native app obtains a **user token for Workflow** through AuthD login.
- The enrollment worker obtains a **service token for APID** through AuthD's
  service-account exchange and renews it without the operator staying online.

The user signs in once and controls all actions through Workflow. Their JWT is
not forwarded into the stitcher, S3 or the enrollment queue. No multi-audience
JWT, shared API audience or delegated user refresh token is required by this
model. Never relax audience validation to accept an APID-only token in Workflow.
An approved client/audience registration and real issuance/renewal proof are
still required before connecting a target environment.

For the recommended one-login target, Workflow is the authenticated upload-signing
entry point as well as the run-control API. The legacy Google-only upload flow
must be adapted or replaced in its upload slice; an AuthD token cannot simply be
sent to it unchanged. This transition is required before claiming one-login
capture/upload, not an implementation already present.

## Documentation alignment and implementation boundaries

- `README.md`, `DESKTOP_APP_PLAN.md`, `SYSTEM_DESIGN.md`, `IMPLEMENTATION_SLICES.md`,
  `SOURCES.md` and the S00/S01a tickets now reflect one AuthD login, accountable
  human ownership, separate execution identity and the next local increment.
  Historical Google capture-source evidence is retained and labeled as legacy.
- [S01a project listing](S01a-authenticated-projects.md) is the
  implementation/acceptance ticket; this file remains the identity decision record.
- Next implementation increment: `apps/workflow/internal/config/`, a new
  `internal/auth/`, `internal/database/queries/`, `internal/httpapi/`, `pkg/api/`
  and `pkg/client/` for identity and authorized context only.
- Later existing slices: run registry, storage/upload signing, scheduler/progress,
  result importer, approval and enrollment modules; native Rust login/client and
  web presentation after their nonvisual proofs. Do not build all of this in one
  authentication change. No capture/stitcher reference checkout edits.
- AuthD client/audience registration needs an owner-reviewed configuration change
  in the real AuthD repository/environment, not changes to a study checkout.

## Reuse

- Existing Rust stitcher engine and reviewed input/output/profile contracts;
  expose processing/progress through an adapter without reimplementing the
  algorithm or putting orchestration/APID enrollment inside it. Deployment
  examples are reference evidence only, not dependencies of this auth increment.
- Existing AuthD human OAuth/PKCE and service-account token contracts; Go remains
  a resource server, not a new identity provider.
- Workflow's `internal/database/queries/` and generated `dbsql` package for
  issuer/subject project memberships; pgx pooling and Goose migrations remain.
- `apps/workflow/internal/httpapi/server.go::NewHandler`, `pkg/api` probe/readiness
  contracts and `pkg/client`'s bounded requests, validation and redirect refusal. Extend these
  rather than introducing a parallel server/client or a new identity provider.

## Steps — next bounded implementation increment

The [S01a ticket](S01a-authenticated-projects.md) is the single scope/acceptance
reference: authenticated project **listing only**, reusing existing schema and
adding one Go client method. Project detail and readiness changes follow later.
Keep maintained JWT/JWKS verification, trusted HTTPS keys, bounded caching and
refresh, claim validation, no redirects/token-supplied key URLs/proxy inheritance,
and no signature/TLS bypass. Optional explicit CA trust supports the local TLS
issuer; no test keys or bypass ship with the application.

All process/database/issuer harnesses, fixtures and reports belong in
`../label-enrollment-harness/`. Source-adjacent unit/contract tests remain with
the application. This decision record defines identity boundaries; do not copy
the ticket checklist or integration logs into it.

### Later prerequisites — not silently waived by this review

- Reconcile the main design/backlog with the subsequent clarification: Workflow
  and stitching are cloud components; their deployment platform remains deferred.
- Qualify the authoritative org/Team permission lookup and project/destination
  mapping before any job/enrollment command is enabled. A readable Team list,
  token org claim or client-supplied context header is insufficient.
- Register/qualify native AuthD login and token renewal in the approved environment.
- Implement run ownership/action audit in the run-registry and approval slices.
  Define/test permission revocation and cancellation before background enrollment.
- Complete upload-auth unification, engine/progress hardening, scoped job launching,
  immutable result import and direct-APID receipts/reconciliation in their existing
  slices. Native/web UI follows nonvisual client proofs, not the reverse.

## Verification required for the applicable implementation slices

- Local actual-service proof with owned PostgreSQL and ephemeral signed-token
  issuer: audience/signature/lifetime validation, project isolation, no spoofed
  header authority and redacted errors/logs. External harness only.
- After deployment selection, a separately authorized nonproduction check must
  prove private/scoped processing invocation and storage allow/deny boundaries.
  Do not require Kubernetes/IRSA to prove application authentication; do not
  embed operator/APID credentials in the stitcher.
- Native login, approval attribution and the upload-login transition each need
  their own proof. Verify cross-org/Team denial, token expiry, membership revocation,
  duplicate starts, event reconnection and continued approved work after UI closure.
- Prove enrollment intent/receipts and ambiguous-outcome reconciliation using
  explicit synthetic fixtures before any separately authorized live integration.
  No live extraction/enrollment or deployment is authorized here.

## Decision recorded

The user accepted the end-to-end flow and clarified that the signed-in person is
responsible for it. They also confirmed **cloud Workflow + cloud stitcher**, with
deployment deferred and no Kubernetes Job requirement. This plan makes human
identity durable while distinguishing service-account execution. It does not
claim implemented authentication, live AuthD/APID compatibility, verified cloud
deployment or scientific acceptance.
The read-only authentication/project-discovery increment above is complete.
Current implementation/proof status is maintained in ORCH-01; the remaining
native, cloud and enrollment flow stays separately gated.
