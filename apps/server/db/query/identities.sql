-- name: GetIdentity :one
SELECT * FROM user_identities
WHERE provider = $1 AND provider_id = $2;

-- name: CreateIdentity :one
INSERT INTO user_identities (user_id, provider, provider_id, name, avatar_url)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListUserIdentities :many
SELECT * FROM user_identities
WHERE user_id = $1
ORDER BY created_at;

-- name: DeleteUserIdentity :execrows
DELETE FROM user_identities
WHERE user_id = $1 AND provider = $2;

-- name: CountIdentitiesByProvider :one
SELECT count(*) FROM user_identities
WHERE provider = $1;

-- name: CreateOauthSignupTicket :one
INSERT INTO oauth_signup_tickets (provider, provider_id, name, avatar_url, secret_hash, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ConsumeOauthSignupTicket :one
UPDATE oauth_signup_tickets
SET consumed_at = now()
WHERE secret_hash = $1 AND consumed_at IS NULL AND expires_at > now()
RETURNING *;

-- name: DeleteExpiredOauthSignupTickets :exec
DELETE FROM oauth_signup_tickets
WHERE expires_at <= now();
