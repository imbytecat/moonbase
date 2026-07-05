-- name: CreateVerificationToken :one
INSERT INTO verification_tokens (purpose, target, user_id, secret_hash, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- Latest usable token for a purpose+target; verification walks newest-first
-- so re-requesting a code invalidates nothing but always wins.
-- name: GetActiveVerificationToken :one
SELECT * FROM verification_tokens
WHERE purpose = $1
  AND target = $2
  AND consumed_at IS NULL
  AND expires_at > now()
ORDER BY created_at DESC
LIMIT 1;

-- name: GetVerificationTokenBySecret :one
SELECT * FROM verification_tokens
WHERE purpose = $1
  AND secret_hash = $2
  AND consumed_at IS NULL
  AND expires_at > now();

-- name: IncrementVerificationAttempts :one
UPDATE verification_tokens
SET attempts = attempts + 1
WHERE id = $1
RETURNING attempts;

-- name: ConsumeVerificationToken :exec
UPDATE verification_tokens
SET consumed_at = now()
WHERE id = $1;

-- Rate-limit input: how many sends for this purpose+target in the window.
-- name: CountRecentVerificationTokens :one
SELECT count(*) FROM verification_tokens
WHERE purpose = $1
  AND target = $2
  AND created_at > $3;

-- name: DeleteExpiredVerificationTokens :exec
DELETE FROM verification_tokens
WHERE expires_at <= now();

-- name: GetUserByPhone :one
SELECT * FROM users
WHERE phone = $1 AND phone <> '';

-- name: SetUserPhone :exec
UPDATE users
SET phone = $2, updated_at = now()
WHERE id = $1;

-- name: SetUserEmailVerified :exec
UPDATE users
SET email_verified_at = now(), updated_at = now()
WHERE id = $1;

-- name: SetUserEmail :exec
UPDATE users
SET email = lower(sqlc.arg('email')), email_verified_at = now(), updated_at = now()
WHERE id = sqlc.arg('id');

-- name: ClearUserPhone :execrows
UPDATE users
SET phone = '', updated_at = now()
WHERE id = $1 AND (username <> '' OR email <> '');

-- name: ClearUserEmail :execrows
UPDATE users
SET email = '', email_verified_at = NULL, updated_at = now()
WHERE id = $1 AND (username <> '' OR phone <> '');
