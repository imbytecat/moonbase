-- +goose Up
ALTER TABLE todos ALTER COLUMN id SET DEFAULT uuidv7();

-- +goose Down
ALTER TABLE todos ALTER COLUMN id SET DEFAULT gen_random_uuid();
