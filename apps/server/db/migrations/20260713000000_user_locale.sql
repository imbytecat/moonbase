-- +goose Up
-- Per-user language preference (BCP-47 tag, e.g. "zh-CN"). Empty = no explicit
-- choice, so clients fall back to browser detection. Stored on the account so
-- the language follows the user across devices, and the backend can localize
-- outbound email/SMS/inbox content to the recipient.
ALTER TABLE users ADD COLUMN locale text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN locale;
