-- Readiness must never call Goose's Status/GetDBVersion: those can initialize
-- its version table. These queries are strictly read-only and bounded.

-- name: HasLegacyMigrationLedger :one
SELECT (to_regclass('workflow.schema_migrations') IS NOT NULL)::boolean AS present;

-- name: ListMigrationVersions :many
SELECT version_id, is_applied
FROM public.workflow_goose_db_version
ORDER BY version_id
LIMIT sqlc.arg(max_versions)::integer;

-- name: CheckProjectSchema :exec
SELECT p.id, p.name, p.created_at,
       m.project_id, m.issuer, m.subject, m.role
FROM workflow.projects AS p
LEFT JOIN workflow.project_memberships AS m ON m.project_id = p.id
LIMIT 0;
