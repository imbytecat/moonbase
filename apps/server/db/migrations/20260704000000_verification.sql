-- +goose Up
ALTER TABLE users
    ADD COLUMN phone text NOT NULL DEFAULT '',
    ADD COLUMN email_verified_at timestamptz;

CREATE UNIQUE INDEX users_phone_idx ON users (phone) WHERE phone <> '';

-- One table for every short-lived secret flow (email verification, password
-- reset, phone binding, SMS login). Only the secret's SHA-256 is stored;
-- attempts guards brute force on 6-digit codes; consumed_at makes tokens
-- single-use.
CREATE TABLE verification_tokens (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    purpose     text        NOT NULL,
    -- Normalized email or phone the secret was sent to.
    target      text        NOT NULL,
    -- NULL for flows that may run pre-login (password reset by email).
    user_id     uuid        REFERENCES users (id) ON DELETE CASCADE,
    secret_hash bytea       NOT NULL,
    attempts    int         NOT NULL DEFAULT 0,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX verification_tokens_lookup_idx
    ON verification_tokens (purpose, target, created_at DESC);
CREATE INDEX verification_tokens_expires_idx ON verification_tokens (expires_at);

-- +goose Down
DROP TABLE IF EXISTS verification_tokens;
DROP INDEX IF EXISTS users_phone_idx;
ALTER TABLE users
    DROP COLUMN IF EXISTS phone,
    DROP COLUMN IF EXISTS email_verified_at;
