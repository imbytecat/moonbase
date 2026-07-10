-- +goose Up
-- Checkout sessions are short-lived interaction state. They select and lock a
-- concrete provider path before producing at most one durable payment order.
CREATE TABLE payment_checkout_sessions (
    id                 text        PRIMARY KEY,
    purpose            text        NOT NULL,
    business_reference text        NOT NULL,
    idempotency_key    text        NOT NULL,
    command_hash       bytea       NOT NULL,
    subject            text        NOT NULL,
    amount             bigint      NOT NULL CHECK (amount > 0),
    currency           text        NOT NULL DEFAULT 'CNY',
    return_path        text        NOT NULL CHECK (return_path LIKE '/%' AND return_path NOT LIKE '//%'),
    status             text        NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'planned', 'confirmed')),
    payment_method     text        NOT NULL DEFAULT '',
    profile_id         text        NOT NULL DEFAULT '',
    profile_name       text        NOT NULL DEFAULT '',
    provider           text        NOT NULL DEFAULT '',
    product_id         text        NOT NULL DEFAULT '',
    expires_at         timestamptz NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (purpose, idempotency_key)
);

ALTER TABLE payment_orders RENAME COLUMN method TO product_id;
ALTER TABLE payment_orders DROP CONSTRAINT payment_orders_status_check;
ALTER TABLE payment_orders
    ADD COLUMN checkout_session_id text REFERENCES payment_checkout_sessions (id),
    ADD COLUMN business_reference text NOT NULL DEFAULT '',
    ADD COLUMN payment_method text NOT NULL DEFAULT '',
    ADD COLUMN inputs jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN action jsonb,
    ADD COLUMN action_expires_at timestamptz,
    ADD COLUMN failure_reason text NOT NULL DEFAULT '';

UPDATE payment_orders
SET payment_method = provider,
    status = CASE WHEN status = 'created' THEN 'pending' ELSE status END;

ALTER TABLE payment_orders ALTER COLUMN status SET DEFAULT 'creating';
ALTER TABLE payment_orders ADD CONSTRAINT payment_orders_status_check
    CHECK (status IN ('creating', 'pending', 'paid', 'failed', 'closed', 'refunding', 'refunded'));
ALTER TABLE payment_orders DROP COLUMN credential;
CREATE UNIQUE INDEX payment_orders_checkout_session_idx
    ON payment_orders (checkout_session_id) WHERE checkout_session_id IS NOT NULL;

-- Settlement delivery is a durable outbox. The order transition and event
-- insert happen in one SQL statement; dispatchers claim rows with SKIP LOCKED
-- and deliver at least once to idempotent business handlers.
CREATE TABLE payment_settlement_events (
    id                 uuid        PRIMARY KEY DEFAULT uuidv7(),
    order_id           uuid        NOT NULL REFERENCES payment_orders (id),
    purpose            text        NOT NULL,
    business_reference text        NOT NULL,
    event_type         text        NOT NULL CHECK (event_type IN ('paid', 'refunded')),
    status             text        NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'delivered')),
    attempts           integer     NOT NULL DEFAULT 0,
    next_attempt_at    timestamptz NOT NULL DEFAULT now(),
    claimed_at         timestamptz,
    delivered_at       timestamptz,
    last_error         text        NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (order_id, event_type)
);

CREATE INDEX payment_settlement_events_dispatch_idx
    ON payment_settlement_events (status, next_attempt_at, created_at);

-- +goose Down
DROP TABLE IF EXISTS payment_settlement_events;
DROP INDEX IF EXISTS payment_orders_checkout_session_idx;
ALTER TABLE payment_orders ADD COLUMN credential text NOT NULL DEFAULT '';
ALTER TABLE payment_orders DROP CONSTRAINT payment_orders_status_check;
ALTER TABLE payment_orders DROP COLUMN failure_reason;
ALTER TABLE payment_orders DROP COLUMN action_expires_at;
ALTER TABLE payment_orders DROP COLUMN action;
ALTER TABLE payment_orders DROP COLUMN inputs;
ALTER TABLE payment_orders DROP COLUMN payment_method;
ALTER TABLE payment_orders DROP COLUMN business_reference;
ALTER TABLE payment_orders DROP COLUMN checkout_session_id;
ALTER TABLE payment_orders RENAME COLUMN product_id TO method;
UPDATE payment_orders SET status = 'created' WHERE status IN ('creating', 'pending', 'failed');
ALTER TABLE payment_orders ALTER COLUMN status SET DEFAULT 'created';
ALTER TABLE payment_orders ADD CONSTRAINT payment_orders_status_check
    CHECK (status IN ('created', 'paid', 'closed', 'refunding', 'refunded'));
DROP TABLE IF EXISTS payment_checkout_sessions;
