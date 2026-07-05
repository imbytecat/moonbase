-- +goose Up
-- Append-only audit trail: mutating RPCs are recorded by a server
-- interceptor. No UPDATE/DELETE path exists in the application; retention is
-- the janitor's job. actor_id has no FK so deleting a user never rewrites
-- history (the name is resolved at read time, best-effort).
CREATE TABLE audit_logs (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    actor_id    uuid,
    action      text        NOT NULL,
    domain      text        NOT NULL,
    resource_id text        NOT NULL DEFAULT '',
    result      text        NOT NULL,
    ip          text        NOT NULL DEFAULT '',
    user_agent  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_created_at_idx ON audit_logs (created_at DESC);
CREATE INDEX audit_logs_actor_idx ON audit_logs (actor_id, created_at DESC);
CREATE INDEX audit_logs_domain_idx ON audit_logs (domain, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
