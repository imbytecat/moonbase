-- +goose Up
-- TOTP second factor. The secret must be stored readable (validation needs
-- it); recovery codes store only SHA-256 hashes. activated_at NULL = setup
-- started but not yet confirmed with a first code.
CREATE TABLE user_mfa (
    user_id        uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    totp_secret    text        NOT NULL,
    recovery_codes bytea[]     NOT NULL DEFAULT '{}',
    activated_at   timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS user_mfa;
