-- +goose Up
-- Payment orders: the durable state machine behind the payment channel.
-- profile_id/profile_name/provider are snapshots (no FK into the JSONB
-- settings) so deleting a profile never rewrites order history. Status
-- transitions are guarded in SQL (WHERE status IN ...) so a replayed
-- provider notification can't regress a settled order.
CREATE TABLE payment_orders (
    id                uuid        PRIMARY KEY DEFAULT uuidv7(),
    out_trade_no      text        NOT NULL UNIQUE,
    purpose           text        NOT NULL,
    profile_id        text        NOT NULL,
    profile_name      text        NOT NULL DEFAULT '',
    provider          text        NOT NULL,
    method            text        NOT NULL,
    subject           text        NOT NULL,
    amount            bigint      NOT NULL CHECK (amount > 0),
    currency          text        NOT NULL DEFAULT 'CNY',
    status            text        NOT NULL DEFAULT 'created'
        CHECK (status IN ('created', 'paid', 'closed', 'refunding', 'refunded')),
    provider_trade_no text        NOT NULL DEFAULT '',
    payer_id          text        NOT NULL DEFAULT '',
    credential        text        NOT NULL DEFAULT '',
    paid_at           timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX payment_orders_created_at_idx ON payment_orders (created_at DESC);
CREATE INDEX payment_orders_status_idx ON payment_orders (status, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS payment_orders;
