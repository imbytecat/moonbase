-- name: InsertCheckoutSession :one
INSERT INTO payment_checkout_sessions (
    id, purpose, business_reference, idempotency_key, command_hash,
    subject, amount, currency, return_path, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (purpose, idempotency_key) DO NOTHING
RETURNING *;

-- name: GetCheckoutSession :one
SELECT * FROM payment_checkout_sessions WHERE id = $1;

-- name: GetCheckoutSessionByIdempotency :one
SELECT * FROM payment_checkout_sessions WHERE purpose = $1 AND idempotency_key = $2;

-- name: PlanCheckoutSession :one
UPDATE payment_checkout_sessions
SET status = 'planned',
    payment_method = $2,
    profile_id = $3,
    profile_name = $4,
    provider = $5,
    product_id = $6,
    updated_at = now()
WHERE id = $1
  AND expires_at > now()
  AND (
    status = 'open'
    OR (
      status = 'planned'
      AND payment_method = $2
      AND profile_id = $3
      AND provider = $5
      AND product_id = $6
    )
  )
RETURNING *;

-- name: StartCheckoutOrder :one
WITH locked_session AS (
    SELECT *
    FROM payment_checkout_sessions
    WHERE payment_checkout_sessions.id = $1
      AND payment_checkout_sessions.status IN ('planned', 'confirmed')
      AND payment_checkout_sessions.expires_at > now()
    FOR UPDATE
), inserted AS (
    INSERT INTO payment_orders (
        checkout_session_id, out_trade_no, purpose, business_reference,
        profile_id, profile_name, provider, payment_method, product_id,
        subject, amount, currency, status, inputs
    )
    SELECT
        id, $2, purpose, business_reference,
        profile_id, profile_name, provider, payment_method, product_id,
        subject, amount, currency, 'creating', $3
    FROM locked_session
    ON CONFLICT (checkout_session_id) WHERE checkout_session_id IS NOT NULL DO NOTHING
    RETURNING *
), confirmed AS (
    UPDATE payment_checkout_sessions
    SET status = 'confirmed', updated_at = now()
    WHERE id IN (SELECT id FROM locked_session)
    RETURNING id
), existing AS (
    SELECT payment_orders.*
    FROM payment_orders
    JOIN locked_session ON locked_session.id = payment_orders.checkout_session_id
)
SELECT * FROM inserted
UNION ALL
SELECT * FROM existing
LIMIT 1;

-- name: GetPaymentOrder :one
SELECT * FROM payment_orders WHERE id = $1;

-- name: GetPaymentOrderByOutTradeNo :one
SELECT * FROM payment_orders WHERE out_trade_no = $1;

-- name: GetPaymentOrderByCheckoutSession :one
SELECT * FROM payment_orders WHERE checkout_session_id = $1;

-- name: SetPaymentOrderPending :one
UPDATE payment_orders
SET status = 'pending', action = $2, action_expires_at = $3, updated_at = now()
WHERE id = $1 AND status = 'creating'
RETURNING *;

-- name: MarkPaymentOrderFailed :one
UPDATE payment_orders
SET status = 'failed', failure_reason = $2, updated_at = now()
WHERE id = $1 AND status = 'creating'
RETURNING *;

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

-- name: MarkPaymentOrderPaid :one
WITH updated AS (
    UPDATE payment_orders
    SET status = 'paid', provider_trade_no = $2, payer_id = $3, paid_at = $4, updated_at = now()
    WHERE payment_orders.id = $1 AND payment_orders.status IN ('creating', 'pending')
    RETURNING *
), event AS (
    INSERT INTO payment_settlement_events (order_id, purpose, business_reference, event_type)
    SELECT id, purpose, business_reference, 'paid' FROM updated
    ON CONFLICT (order_id, event_type) DO NOTHING
)
SELECT id FROM updated;

-- name: MarkPaymentOrderClosed :one
UPDATE payment_orders
SET status = 'closed', updated_at = now()
WHERE id = $1 AND status IN ('creating', 'pending')
RETURNING *;

-- name: MarkPaymentOrderRefunding :one
UPDATE payment_orders
SET status = 'refunding', updated_at = now()
WHERE id = $1 AND status = 'paid'
RETURNING *;

-- name: MarkPaymentOrderRefunded :one
WITH updated AS (
    UPDATE payment_orders
    SET status = 'refunded', updated_at = now()
    WHERE payment_orders.id = $1 AND payment_orders.status IN ('paid', 'refunding')
    RETURNING *
), event AS (
    INSERT INTO payment_settlement_events (order_id, purpose, business_reference, event_type)
    SELECT id, purpose, business_reference, 'refunded' FROM updated
    ON CONFLICT (order_id, event_type) DO NOTHING
)
SELECT id FROM updated;

-- name: ClaimSettlementEvent :one
WITH candidate AS (
    SELECT id
    FROM payment_settlement_events
    WHERE (
        status = 'pending'
        OR (status = 'processing' AND claimed_at < now() - interval '5 minutes')
    )
      AND next_attempt_at <= now()
    ORDER BY created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE payment_settlement_events
SET status = 'processing', attempts = attempts + 1, claimed_at = now(), last_error = ''
WHERE id = (SELECT id FROM candidate)
RETURNING *;

-- name: MarkSettlementEventDelivered :exec
UPDATE payment_settlement_events
SET status = 'delivered', delivered_at = now(), claimed_at = NULL
WHERE id = $1 AND status = 'processing';

-- name: RetrySettlementEvent :exec
UPDATE payment_settlement_events
SET status = 'pending', next_attempt_at = $2, claimed_at = NULL, last_error = $3
WHERE id = $1 AND status = 'processing';
