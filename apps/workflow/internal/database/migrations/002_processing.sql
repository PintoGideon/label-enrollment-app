-- +goose Up
-- Aggregate snapshots are versioned contracts; commands/events/results are
-- immutable history. Infrastructure leases never substitute for human authority.
CREATE TABLE workflow.processing_runs (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES workflow.projects(id) ON DELETE RESTRICT,
    revision bigint NOT NULL CHECK (revision > 0),
    state text NOT NULL CHECK (state IN ('registered','verifying','verified','queued','running','validating','cancelling','uncertain','cancelled','failed','reviewable','approved')),
    owner_issuer text NOT NULL,
    owner_subject text NOT NULL,
    snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
    lease_owner uuid,
    lease_until timestamptz,
    fence bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, id)
);
CREATE INDEX processing_runs_project ON workflow.processing_runs(project_id, id);
CREATE INDEX processing_runs_pending ON workflow.processing_runs(lease_until, updated_at)
    WHERE state IN ('verifying','queued','running','validating','cancelling','uncertain');
CREATE TABLE workflow.processing_commands (
    project_id uuid NOT NULL REFERENCES workflow.projects(id) ON DELETE RESTRICT,
    issuer text NOT NULL,
    subject text NOT NULL,
    command_id uuid NOT NULL,
    request_digest text NOT NULL CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    response jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(project_id, issuer, subject, command_id)
);
CREATE TABLE workflow.processing_events (
    sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES workflow.processing_runs(id) ON DELETE RESTRICT,
    revision bigint NOT NULL,
    event jsonb NOT NULL,
    UNIQUE (run_id, revision)
);
CREATE INDEX processing_events_run ON workflow.processing_events(run_id, sequence);
CREATE TABLE workflow.processing_attempts (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES workflow.processing_runs(id) ON DELETE RESTRICT,
    number integer NOT NULL CHECK (number BETWEEN 1 AND 20),
    intent jsonb NOT NULL,
    UNIQUE(run_id,number)
);
CREATE TABLE workflow.processing_results (
    attempt_id uuid PRIMARY KEY REFERENCES workflow.processing_attempts(id) ON DELETE RESTRICT,
    result jsonb NOT NULL
);
CREATE TABLE workflow.processing_reviews (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES workflow.processing_runs(id) ON DELETE RESTRICT,
    review jsonb NOT NULL
);
CREATE TABLE workflow.processing_approvals (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES workflow.processing_runs(id) ON DELETE RESTRICT,
    approval jsonb NOT NULL
);
