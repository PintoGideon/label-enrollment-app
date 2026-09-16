-- name: ListAuthorizedProjects :many
SELECT p.id, p.name,
    array_agg(m.role ORDER BY CASE m.role
        WHEN 'capture' THEN 0 WHEN 'process' THEN 1
        WHEN 'review' THEN 2 WHEN 'enroll' THEN 3 END)::text[] AS roles
FROM workflow.projects AS p
JOIN workflow.project_memberships AS m ON m.project_id = p.id
WHERE m.issuer = sqlc.arg(issuer)
    AND m.subject = sqlc.arg(subject)
    AND (sqlc.narg(cursor)::uuid IS NULL OR p.id > sqlc.narg(cursor)::uuid)
GROUP BY p.id, p.name
ORDER BY p.id
LIMIT sqlc.arg(page_size);
