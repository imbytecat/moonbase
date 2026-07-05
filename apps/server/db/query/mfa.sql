-- name: GetUserMfa :one
SELECT * FROM user_mfa
WHERE user_id = $1;

-- name: UpsertPendingUserMfa :execrows
INSERT INTO user_mfa (user_id, totp_secret, recovery_codes)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE
SET totp_secret = excluded.totp_secret,
    recovery_codes = excluded.recovery_codes,
    created_at = now()
WHERE user_mfa.activated_at IS NULL;

-- name: ActivateUserMfa :execrows
UPDATE user_mfa
SET activated_at = now()
WHERE user_id = $1 AND activated_at IS NULL;

-- name: DeleteUserMfa :execrows
DELETE FROM user_mfa
WHERE user_id = $1;

-- name: ConsumeMfaRecoveryCode :execrows
UPDATE user_mfa
SET recovery_codes = array_remove(recovery_codes, sqlc.arg('code_hash')::bytea)
WHERE user_id = sqlc.arg('user_id')
  AND sqlc.arg('code_hash')::bytea = ANY (recovery_codes);
