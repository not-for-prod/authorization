-- +goose Up
CREATE TABLE auth_user (
    id text PRIMARY KEY,
    status text NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE TABLE auth_identity (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES auth_user(id),
    kind text NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    password_hash text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    UNIQUE (issuer, subject)
);
CREATE INDEX auth_identity_user_idx ON auth_identity (user_id);
CREATE TABLE auth_transaction (
    tenant_id text NOT NULL,
    id text NOT NULL,
    owner_key text NOT NULL,
    snapshot jsonb NOT NULL,
    PRIMARY KEY (tenant_id, id),
    CHECK (jsonb_typeof(snapshot) = 'object')
);
-- +goose Down
DROP TABLE auth_transaction;
DROP TABLE auth_identity;
DROP TABLE auth_user;
