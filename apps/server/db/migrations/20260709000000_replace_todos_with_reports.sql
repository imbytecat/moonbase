-- +goose Up

-- The todo CRUD example is replaced by the report domain, which aggregates
-- real system data instead of demo rows. Drop the demo table and migrate any
-- stored role grants: either todo permission becomes report.read (the report
-- surface is read-only, so todo.write has no successor).
DROP TABLE IF EXISTS todos;

INSERT INTO role_permissions (role_id, permission)
SELECT DISTINCT role_id, 'report.read'
FROM role_permissions
WHERE permission IN ('todo.read', 'todo.write')
ON CONFLICT DO NOTHING;

DELETE FROM role_permissions
WHERE permission IN ('todo.read', 'todo.write');

-- +goose Down
DELETE FROM role_permissions WHERE permission = 'report.read';

CREATE TABLE todos (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    title      text        NOT NULL,
    completed  boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
