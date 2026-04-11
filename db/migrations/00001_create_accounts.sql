-- +goose Up
CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    opening_balance INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS accounts;
