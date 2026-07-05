-- name: InsertFile :one
INSERT INTO files (object_key, content_type, uploaded_by, purpose)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetFile :one
SELECT * FROM files WHERE id = $1;
