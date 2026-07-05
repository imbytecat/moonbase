-- +goose Up
CREATE TABLE IF NOT EXISTS todos (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    title      text        NOT NULL,
    completed  boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS todos_created_at_idx ON todos (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS todos;
