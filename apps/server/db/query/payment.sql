-- name: InsertPaymentOrder :one
INSERT INTO payment_orders (out_trade_no, purpose, profile_id, profile_name, provider, method, subject, amount, currency, credential)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetPaymentOrder :one
SELECT * FROM payment_orders WHERE id = $1;

-- name: GetPaymentOrderByOutTradeNo :one
SELECT * FROM payment_orders WHERE out_trade_no = $1;

-- name: ListPaymentOrders :many
SELECT * FROM payment_orders
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('provider')::text IS NULL OR provider = sqlc.narg('provider'))
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountPaymentOrders :one
SELECT count(*) FROM payment_orders
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('provider')::text IS NULL OR provider = sqlc.narg('provider'));

-- The status guard makes settlement idempotent: a replayed notification or a
-- concurrent sync matches zero rows once the order left 'created'.
-- name: MarkPaymentOrderPaid :one
UPDATE payment_orders
SET status = 'paid', provider_trade_no = $2, payer_id = $3, paid_at = $4, updated_at = now()
WHERE id = $1 AND status = 'created'
RETURNING *;

-- name: MarkPaymentOrderClosed :one
UPDATE payment_orders
SET status = 'closed', updated_at = now()
WHERE id = $1 AND status = 'created'
RETURNING *;

-- name: MarkPaymentOrderRefunding :one
UPDATE payment_orders
SET status = 'refunding', updated_at = now()
WHERE id = $1 AND status = 'paid'
RETURNING *;

-- name: MarkPaymentOrderRefunded :one
UPDATE payment_orders
SET status = 'refunded', updated_at = now()
WHERE id = $1 AND status IN ('paid', 'refunding')
RETURNING *;
