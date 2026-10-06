-- +goose Up
CREATE TABLE auth_completion (
    tenant_id text NOT NULL,
    id text NOT NULL,
    owner_key text NOT NULL,
    receipt jsonb NOT NULL CHECK (jsonb_typeof(receipt) = 'object'),
    PRIMARY KEY (tenant_id, id)
);
-- +goose Down
DROP TABLE auth_completion;
