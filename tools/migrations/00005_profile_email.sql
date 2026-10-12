-- +goose Up
ALTER TABLE auth_identity ADD COLUMN verified_email text NOT NULL DEFAULT '';
ALTER TABLE auth_identity ADD CONSTRAINT auth_identity_verified_email_length CHECK (length(verified_email) <= 254);

-- +goose Down
ALTER TABLE auth_identity DROP COLUMN verified_email;
