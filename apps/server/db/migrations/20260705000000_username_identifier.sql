-- +goose Up
ALTER TABLE users
    ADD COLUMN username text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX users_username_idx ON users (lower(username)) WHERE username <> '';

-- Email becomes an optional identifier (same '' sentinel + partial unique
-- pattern as phone): mainland-China deployments often have no email at all,
-- and forcing one just produces fake addresses.
DROP INDEX users_email_idx;
CREATE UNIQUE INDEX users_email_idx ON users (lower(email)) WHERE email <> '';

-- Every account must stay reachable by at least one login identifier.
ALTER TABLE users
    ADD CONSTRAINT users_has_identifier CHECK (username <> '' OR email <> '' OR phone <> '');

-- +goose Down
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_has_identifier;
DROP INDEX IF EXISTS users_email_idx;
CREATE UNIQUE INDEX users_email_idx ON users (lower(email));
DROP INDEX IF EXISTS users_username_idx;
ALTER TABLE users DROP COLUMN IF EXISTS username;
