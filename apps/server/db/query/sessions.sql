-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- Resolve a session token to a full identity in one round trip: user fields
-- plus the union of permissions from every role the user holds. Expired
-- sessions and disabled users resolve to no rows (treated as unauthenticated).
-- name: GetSessionIdentity :one
SELECT
    s.id AS session_id,
    s.expires_at,
    s.created_at AS session_created_at,
    u.id AS user_id,
    u.username,
    u.email,
    u.name,
    coalesce(f.object_key, '')::text AS avatar_key,
    u.phone,
    u.locale,
    (u.email_verified_at IS NOT NULL)::bool AS email_verified,
    coalesce(
        array_agg(DISTINCT rp.permission) FILTER (WHERE rp.permission IS NOT NULL),
        '{}'
    )::text[] AS permissions
FROM sessions s
JOIN users u ON u.id = s.user_id
LEFT JOIN files f ON f.id = u.avatar_file_id
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN role_permissions rp ON rp.role_id = ur.role_id
WHERE s.token_hash = $1
  AND s.expires_at > now()
  AND u.is_active
GROUP BY s.id, u.id, f.object_key;

-- name: TouchSession :exec
UPDATE sessions
SET expires_at = $2
WHERE id = $1;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE id = $1;

-- name: DeleteSessionByTokenHash :exec
DELETE FROM sessions
WHERE token_hash = $1;

-- name: DeleteOtherUserSessions :exec
DELETE FROM sessions
WHERE user_id = $1 AND id <> $2;

-- name: DeleteUserSessions :exec
DELETE FROM sessions
WHERE user_id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= now();

-- name: ListUserSessions :many
SELECT * FROM sessions
WHERE user_id = $1 AND expires_at > now()
ORDER BY created_at DESC;

-- name: DeleteUserSessionByID :execrows
DELETE FROM sessions
WHERE id = $1 AND user_id = $2;
