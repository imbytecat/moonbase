-- +goose Up
ALTER TABLE sessions
    ADD COLUMN user_agent text NOT NULL DEFAULT '',
    ADD COLUMN ip         text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE sessions
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS ip;
