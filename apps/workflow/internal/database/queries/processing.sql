-- name: LockProcessingProject :one
SELECT id FROM workflow.projects WHERE id = $1 FOR UPDATE;

-- name: ProcessingRoles :many
SELECT role FROM workflow.project_memberships
WHERE project_id = $1 AND issuer = $2 AND subject = $3 FOR SHARE;

-- name: FindProcessingCommand :one
SELECT request_digest, response FROM workflow.processing_commands
WHERE project_id = $1 AND issuer = $2 AND subject = $3 AND command_id = $4;

-- name: SaveProcessingCommand :exec
INSERT INTO workflow.processing_commands(project_id,issuer,subject,command_id,request_digest,response)
VALUES ($1,$2,$3,$4,$5,$6);

-- name: ReadProcessingRun :one
SELECT snapshot FROM workflow.processing_runs WHERE project_id=$1 AND id=$2;

-- name: LockProcessingRun :one
SELECT snapshot FROM workflow.processing_runs WHERE project_id=$1 AND id=$2 FOR UPDATE;

-- name: ListProcessingRuns :many
SELECT snapshot FROM workflow.processing_runs
WHERE project_id=$1 AND (sqlc.narg(cursor)::uuid IS NULL OR id > sqlc.narg(cursor)::uuid)
ORDER BY id LIMIT 11;

-- name: CreateProcessingRun :exec
INSERT INTO workflow.processing_runs(id,project_id,revision,state,owner_issuer,owner_subject,snapshot)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: UpdateProcessingRun :execrows
UPDATE workflow.processing_runs SET revision=revision+1,state=$3,snapshot=$4,updated_at=now()
WHERE id=$1 AND revision=$2;

-- name: SaveProcessingEvent :exec
INSERT INTO workflow.processing_events(run_id,revision,event) VALUES($1,$2,$3);

-- name: ListProcessingEvents :many
SELECT sequence,event FROM workflow.processing_events WHERE run_id=$1 AND sequence>$2 ORDER BY sequence LIMIT 101;

-- name: SaveProcessingAttempt :exec
INSERT INTO workflow.processing_attempts(id,run_id,number,intent) VALUES($1,$2,$3,$4);

-- name: SaveProcessingResult :exec
INSERT INTO workflow.processing_results(attempt_id,result) VALUES($1,$2);

-- name: SaveProcessingReview :exec
INSERT INTO workflow.processing_reviews(id,run_id,review) VALUES($1,$2,$3);

-- name: SaveProcessingApproval :exec
INSERT INTO workflow.processing_approvals(id,run_id,approval) VALUES($1,$2,$3);

-- name: ReadProcessingResult :one
SELECT r.result FROM workflow.processing_results r
JOIN workflow.processing_attempts a ON a.id=r.attempt_id
WHERE a.run_id=$1 AND a.id=$2;

-- name: ReadProcessingApproval :one
SELECT approval FROM workflow.processing_approvals WHERE run_id=$1 AND id=$2;

-- name: ClaimProcessingRun :one
UPDATE workflow.processing_runs SET lease_owner=$1,lease_until=now()+interval '60 seconds',fence=fence+1
WHERE id=(SELECT id FROM workflow.processing_runs
 WHERE state IN ('verifying','queued','running','validating','cancelling','uncertain')
 AND (lease_until IS NULL OR lease_until<now())
 ORDER BY updated_at FOR UPDATE SKIP LOCKED LIMIT 1)
RETURNING snapshot,fence;

-- name: RenewProcessingLease :execrows
UPDATE workflow.processing_runs SET lease_until=now()+interval '60 seconds'
WHERE id=$1 AND lease_owner=$2 AND fence=$3 AND lease_until>now() AND revision=$4;

-- name: ReleaseProcessingLease :exec
UPDATE workflow.processing_runs SET lease_owner=NULL,lease_until=NULL,updated_at=now()
WHERE id=$1 AND lease_owner=$2 AND fence=$3;

-- name: CheckProcessingLease :one
SELECT EXISTS(SELECT 1 FROM workflow.processing_runs
 WHERE id=$1 AND lease_owner=$2 AND fence=$3 AND lease_until>now());

-- name: CheckProcessingSchema :exec
SELECT r.id,r.project_id,r.revision,r.state,r.owner_issuer,r.owner_subject,r.snapshot,r.lease_owner,r.lease_until,r.fence,
 c.command_id,c.request_digest,c.response,e.sequence,e.event,a.intent,s.result,v.review,p.approval
FROM workflow.processing_runs r,workflow.processing_commands c,workflow.processing_events e,
workflow.processing_attempts a,workflow.processing_results s,workflow.processing_reviews v,workflow.processing_approvals p LIMIT 0;
