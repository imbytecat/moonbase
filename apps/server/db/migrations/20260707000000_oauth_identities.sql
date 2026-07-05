-- +goose Up
-- Third-party login identities (WeChat, and future OAuth providers), the
-- Supabase identities pattern: identity rows are decoupled from the user row
-- so one account can hold many providers and new providers need no users DDL.
CREATE TABLE user_identities (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider    text        NOT NULL,
    -- Stable subject from the provider: WeChat unionid (openid fallback).
    provider_id text        NOT NULL,
    name        text        NOT NULL DEFAULT '',
    avatar_url  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_id),
    UNIQUE (user_id, provider)
);

-- Pending social signups: the callback verified the external identity but no
-- local account exists yet; the completion form consumes the ticket.
CREATE TABLE oauth_signup_tickets (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    provider    text        NOT NULL,
    provider_id text        NOT NULL,
    name        text        NOT NULL DEFAULT '',
    avatar_url  text        NOT NULL DEFAULT '',
    secret_hash bytea       NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS oauth_signup_tickets;
DROP TABLE IF EXISTS user_identities;
