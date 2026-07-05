-- name: InsertFile :one
INSERT INTO files (object_key, content_type, uploaded_by, purpose)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetFile :one
SELECT * FROM files WHERE id = $1;

-- name: ListUnattachedFiles :many
-- The unattached-file sweep's one query (ADR-0003): files created before the
-- grace cutoff that no attachment references. purpose rides along so the sweep
-- can resolve which backend to delete each object from.
SELECT id, object_key, purpose FROM files f
WHERE f.created_at < sqlc.arg('created_before')
  AND NOT EXISTS (SELECT 1 FROM file_attachments fa WHERE fa.file_id = f.id)
ORDER BY f.created_at;

-- name: DeleteFile :exec
DELETE FROM files WHERE id = $1;
