-- name: InsertNotification :exec
INSERT INTO notifications (user_id, category, title, body, link)
VALUES ($1, $2, $3, $4, $5);

-- Recipients of a permission (through any role, or the '*' admin wildcard).
-- name: ListUsersForPermission :many
SELECT DISTINCT u.id
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN role_permissions rp ON rp.role_id = ur.role_id
WHERE rp.permission = $1 OR rp.permission = '*';

-- unread_only false returns the whole inbox; true narrows to still-unread.
-- name: ListNotifications :many
SELECT *
FROM notifications
WHERE user_id = sqlc.arg('user_id')
  AND (NOT sqlc.arg('unread_only')::boolean OR read_at IS NULL)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountNotifications :one
SELECT count(*)
FROM notifications
WHERE user_id = sqlc.arg('user_id')
  AND (NOT sqlc.arg('unread_only')::boolean OR read_at IS NULL);

-- name: CountUnreadNotifications :one
SELECT count(*)
FROM notifications
WHERE user_id = $1 AND read_at IS NULL;

-- All mutations are scoped by user_id so no caller can touch another inbox;
-- only unread rows are stamped, preserving the first read time.
-- name: MarkNotificationsRead :exec
UPDATE notifications
SET read_at = now()
WHERE user_id = sqlc.arg('user_id') AND id = ANY(sqlc.arg('ids')::uuid[]) AND read_at IS NULL;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications
SET read_at = now()
WHERE user_id = $1 AND read_at IS NULL;

-- name: DeleteNotification :exec
DELETE FROM notifications
WHERE user_id = $1 AND id = $2;
