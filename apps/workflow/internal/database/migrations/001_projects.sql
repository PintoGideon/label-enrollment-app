-- +goose Up
-- Application schema only: no test data, default principals or APID destinations.
CREATE SCHEMA IF NOT EXISTS workflow;

CREATE TABLE workflow.projects (
    id uuid PRIMARY KEY,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200 AND length(btrim(name)) > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workflow.project_memberships (
    project_id uuid NOT NULL REFERENCES workflow.projects(id) ON DELETE RESTRICT,
    issuer text NOT NULL CHECK (length(issuer) BETWEEN 1 AND 512),
    subject text NOT NULL CHECK (length(subject) BETWEEN 1 AND 255),
    role text NOT NULL CHECK (role IN ('capture', 'process', 'review', 'enroll')),
    PRIMARY KEY (project_id, issuer, subject, role)
);

CREATE INDEX project_memberships_principal
    ON workflow.project_memberships (issuer, subject, project_id);
