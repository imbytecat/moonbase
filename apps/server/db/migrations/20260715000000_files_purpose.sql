-- +goose Up
-- files.purpose records which storage purpose holds the object, and thus which
-- bound profile/driver it lives behind. The unattached-file sweep (ADR-0003)
-- deletes objects via ObjectStore.Delete(purpose, key); object_key alone does
-- not say where the object lives, so the ledger row must carry the purpose the
-- presign RPC resolved at upload time. Backfill existing rows from the key
-- prefix the presign RPCs mint.
ALTER TABLE files ADD COLUMN purpose text NOT NULL DEFAULT '';

UPDATE files SET purpose = CASE
    WHEN object_key LIKE 'avatars/%' THEN 'avatars'
    WHEN object_key LIKE 'site/%'    THEN 'site-assets'
    ELSE purpose
END;

-- +goose Down
ALTER TABLE files DROP COLUMN purpose;
