-- Aggregation queries for report.v1. All heavy lifting stays in SQL so the
-- RPC layer only maps rows to proto — never streams raw rows to count them.

-- name: CountActiveUsers :one
SELECT count(*) FROM users WHERE is_active;

-- name: CountUsersCreatedSince :one
SELECT count(*) FROM users
WHERE created_at >= now() - make_interval(days => sqlc.arg('days')::int);

-- name: CountActiveSessions :one
SELECT count(*) FROM sessions WHERE expires_at > now();

-- name: UserSignupsByDay :many
SELECT created_at::date AS day, count(*) AS count
FROM users
WHERE created_at >= now() - make_interval(days => sqlc.arg('days')::int)
GROUP BY day
ORDER BY day;

-- name: LoginsByDay :many
SELECT created_at::date AS day, count(*) AS count
FROM sessions
WHERE created_at >= now() - make_interval(days => sqlc.arg('days')::int)
GROUP BY day
ORDER BY day;

-- name: IdentitiesByProvider :many
SELECT provider, count(*) AS count
FROM user_identities
GROUP BY provider
ORDER BY count DESC;

-- name: UsersByRole :many
SELECT r.name, count(ur.user_id) AS count
FROM roles r
LEFT JOIN user_roles ur ON ur.role_id = r.id
GROUP BY r.id, r.name
ORDER BY count DESC, r.name;
