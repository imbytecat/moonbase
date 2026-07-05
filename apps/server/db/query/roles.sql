-- name: CreateRole :one
INSERT INTO roles (name, description, is_system)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetRole :one
SELECT * FROM roles
WHERE id = $1;

-- name: GetRoleByName :one
SELECT * FROM roles
WHERE name = $1;

-- name: ListRoles :many
SELECT * FROM roles
ORDER BY created_at;

-- name: UpdateRole :one
UPDATE roles
SET name        = coalesce(sqlc.narg('name'), name),
    description = coalesce(sqlc.narg('description'), description),
    updated_at  = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteRole :exec
DELETE FROM roles
WHERE id = $1 AND NOT is_system;

-- name: ListRolePermissions :many
SELECT role_id, permission FROM role_permissions
ORDER BY permission;

-- name: DeleteRolePermissions :exec
DELETE FROM role_permissions
WHERE role_id = $1;

-- name: AddRolePermissions :exec
INSERT INTO role_permissions (role_id, permission)
SELECT sqlc.arg('role_id'), unnest(sqlc.arg('permissions')::text[])
ON CONFLICT DO NOTHING;

-- name: CountUsersWithRole :one
SELECT count(*) FROM user_roles
WHERE role_id = $1;
