-- +goose Up
-- The file ledger: one row per object the system accounts for (ADR-0003).
-- Written at presign time so every object that could exist has a record from
-- the first second — an object in the bucket with no files row is impossible
-- by construction. A row is spiritually immutable: replacing a file means a
-- new row, never an update. uploaded_by is a plain stamp of who authorized the
-- upload (no FK — deleting the user must not be blocked, and the row is
-- reclaimed by the unattached sweep, not cascaded).
CREATE TABLE files (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    object_key   text        NOT NULL,
    content_type text        NOT NULL,
    uploaded_by  uuid        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- file_attachments: a polymorphic reference from a domain entity to a file
-- (owner_type + owner_id -> file_id). The foreign key is the whole point — a
-- referenced file cannot be deleted, so a dangling reference is impossible by
-- construction. A file may be referenced from several places; the unattached
-- sweep reclaims it once its references reach zero.
CREATE TABLE file_attachments (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    file_id    uuid        NOT NULL REFERENCES files (id),
    owner_type text        NOT NULL,
    owner_id   text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS file_attachments;
DROP TABLE IF EXISTS files;
