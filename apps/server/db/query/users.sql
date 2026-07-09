-- name: CreateUser :one
INSERT INTO users (username, email, name, password_hash, phone, email_verified_at)
VALUES (
    sqlc.arg('username'),
    lower(sqlc.arg('email')),
    sqlc.arg('name'),
    sqlc.arg('password_hash'),
    sqlc.arg('phone'),
    CASE WHEN sqlc.arg('email_verified')::bool THEN now() END
)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = lower(sqlc.arg('email')) AND email <> '';

-- name: GetUserByUsername :one
SELECT * FROM users
WHERE lower(username) = lower(sqlc.arg('username')) AND username <> '';

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: ListUsers :many
SELECT * FROM users
ORDER BY created_at DESC;

-- name: UpdateUser :one
UPDATE users
SET email      = coalesce(lower(sqlc.narg('email')), email),
    name       = coalesce(sqlc.narg('name'), name),
    is_active  = coalesce(sqlc.narg('is_active'), is_active),
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetUserAvatar :exec
-- Transfer the user's single avatar slot to file_id (NULL clears it) in one
-- atomic statement: drop the old attachment, add the new one, and repoint the
-- user. The old file loses its last attachment and becomes unattached, so the
-- sweep reclaims it (ADR-0003) — no explicit delete here.
WITH cleared AS (
    DELETE FROM file_attachments
    WHERE owner_type = 'user' AND owner_id = sqlc.arg('user_id')::uuid::text
),
attached AS (
    INSERT INTO file_attachments (file_id, owner_type, owner_id)
    SELECT sqlc.narg('file_id')::uuid, 'user', sqlc.arg('user_id')::uuid::text
    WHERE sqlc.narg('file_id') IS NOT NULL
)
UPDATE users
SET avatar_file_id = sqlc.narg('file_id')::uuid, updated_at = now()
WHERE users.id = sqlc.arg('user_id')::uuid;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2, updated_at = now()
WHERE id = $1;

-- name: DeleteUser :exec
DELETE FROM users
WHERE id = $1;

-- name: ListUserRolesWithIDs :many
SELECT ur.user_id, r.id AS role_id, r.name
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
ORDER BY r.name;

-- name: DeleteUserRoles :exec
DELETE FROM user_roles
WHERE user_id = $1;

-- name: AddUserRoles :exec
INSERT INTO user_roles (user_id, role_id)
SELECT sqlc.arg('user_id'), unnest(sqlc.arg('role_ids')::uuid[])
ON CONFLICT DO NOTHING;
