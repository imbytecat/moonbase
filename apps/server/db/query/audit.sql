-- name: InsertAuditLog :exec
INSERT INTO audit_logs (actor_id, action, domain, resource_id, result, ip, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- Filters are optional: sqlc.narg NULL means "no filter". actor names join
-- best-effort so deleted users still show their recorded actions.
-- name: ListAuditLogs :many
SELECT
    a.*,
    coalesce(u.name, '') AS actor_name
FROM audit_logs a
LEFT JOIN users u ON u.id = a.actor_id
WHERE (sqlc.narg('actor_id')::uuid IS NULL OR a.actor_id = sqlc.narg('actor_id'))
  AND (sqlc.narg('domain')::text IS NULL OR a.domain = sqlc.narg('domain'))
  AND (sqlc.narg('action')::text IS NULL OR a.action = sqlc.narg('action'))
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR a.created_at >= sqlc.narg('from_time'))
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR a.created_at <= sqlc.narg('to_time'))
ORDER BY a.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountAuditLogs :one
SELECT count(*)
FROM audit_logs a
WHERE (sqlc.narg('actor_id')::uuid IS NULL OR a.actor_id = sqlc.narg('actor_id'))
  AND (sqlc.narg('domain')::text IS NULL OR a.domain = sqlc.narg('domain'))
  AND (sqlc.narg('action')::text IS NULL OR a.action = sqlc.narg('action'))
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR a.created_at >= sqlc.narg('from_time'))
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR a.created_at <= sqlc.narg('to_time'));

-- name: DeleteAuditLogsBefore :exec
DELETE FROM audit_logs
WHERE created_at < $1;
