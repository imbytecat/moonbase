-- +goose Up
-- Migrate avatars off the raw object key onto a file-ledger reference
-- (ADR-0003 "存量槽位迁入，不留双轨"). avatar_file_id points at the files row;
-- the file is kept alive by a matching file_attachments row, so a replaced
-- avatar's old file goes unattached and is reclaimed by the sweep.
ALTER TABLE users ADD COLUMN avatar_file_id uuid REFERENCES files (id);

-- Backfill every existing avatar in one pass: mint a files row (purpose
-- avatars, content type inferred from the key's extension, uploader = the owner)
-- and an attachment, then point the user at it. Keys are avatars/<uid>/<rand>,
-- unique per user, so the final join matching users.id = files.uploaded_by is
-- one-to-one.
WITH owned AS (
    SELECT
        id AS user_id,
        avatar_key,
        CASE
            WHEN avatar_key LIKE '%.png'  THEN 'image/png'
            WHEN avatar_key LIKE '%.jpg'  THEN 'image/jpeg'
            WHEN avatar_key LIKE '%.jpeg' THEN 'image/jpeg'
            WHEN avatar_key LIKE '%.webp' THEN 'image/webp'
            ELSE 'application/octet-stream'
        END AS content_type
    FROM users
    WHERE avatar_key <> ''
),
minted AS (
    INSERT INTO files (object_key, content_type, uploaded_by, purpose)
    SELECT avatar_key, content_type, user_id, 'avatars' FROM owned
    RETURNING id, uploaded_by
),
attached AS (
    INSERT INTO file_attachments (file_id, owner_type, owner_id)
    SELECT id, 'user', uploaded_by::text FROM minted
)
UPDATE users u
SET avatar_file_id = m.id
FROM minted m
WHERE u.id = m.uploaded_by;

ALTER TABLE users DROP COLUMN avatar_key;

-- +goose Down
ALTER TABLE users ADD COLUMN avatar_key text NOT NULL DEFAULT '';
UPDATE users u SET avatar_key = f.object_key
FROM files f
WHERE u.avatar_file_id = f.id;
ALTER TABLE users DROP COLUMN avatar_file_id;
