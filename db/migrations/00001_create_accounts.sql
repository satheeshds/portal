-- +goose Up
CREATE SEQUENCE IF NOT EXISTS accounts_id_seq START 1;
CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER NOT NULL DEFAULT nextval('accounts_id_seq'),
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    opening_balance INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS accounts;
DROP SEQUENCE IF EXISTS accounts_id_seq;
