-- +goose Up
-- Per-user in-app inbox (站内信). Rows are addressed to a single user and
-- carry already-rendered title/body (the producer localizes at publish time);
-- listing never needs the producing domain again. read_at NULL = unread.
-- Deleting a user cascades their inbox away.
CREATE TABLE notifications (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    category   text        NOT NULL,
    title      text        NOT NULL,
    body       text        NOT NULL DEFAULT '',
    link       text        NOT NULL DEFAULT '',
    read_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- The inbox is always read newest-first, scoped to one user.
CREATE INDEX notifications_user_created_idx ON notifications (user_id, created_at DESC);
-- Partial index powers the unread badge count without scanning read rows.
CREATE INDEX notifications_user_unread_idx ON notifications (user_id) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS notifications;
