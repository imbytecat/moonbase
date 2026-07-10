-- name: GetSetting :one
SELECT * FROM settings
WHERE key = $1;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value)
VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE
SET value = excluded.value, updated_at = now();

-- name: GetOrCreateSetting :one
INSERT INTO settings (key, value)
VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE
SET value = settings.value
RETURNING *;

-- name: SetSiteWithAssets :exec
-- Save the site settings JSONB and move the logo/favicon attachments to the
-- referenced files in one atomic statement (ADR-0003): each brand slot's old
-- attachment is dropped and re-pointed, so a replaced asset's previous file
-- goes unattached for the sweep. A NULL file id clears that slot.
WITH cleared_logo AS (
    DELETE FROM file_attachments WHERE owner_type = 'site' AND owner_id = 'logo'
),
attached_logo AS (
    INSERT INTO file_attachments (file_id, owner_type, owner_id)
    SELECT sqlc.narg('logo_file_id')::uuid, 'site', 'logo'
    WHERE sqlc.narg('logo_file_id') IS NOT NULL
),
cleared_favicon AS (
    DELETE FROM file_attachments WHERE owner_type = 'site' AND owner_id = 'favicon'
),
attached_favicon AS (
    INSERT INTO file_attachments (file_id, owner_type, owner_id)
    SELECT sqlc.narg('favicon_file_id')::uuid, 'site', 'favicon'
    WHERE sqlc.narg('favicon_file_id') IS NOT NULL
)
INSERT INTO settings (key, value)
VALUES ('site', sqlc.arg('site')::jsonb)
ON CONFLICT (key) DO UPDATE
SET value = excluded.value, updated_at = now();
