-- +goose Up
CREATE SEQUENCE IF NOT EXISTS transactions_id_seq START 1;
CREATE TABLE IF NOT EXISTS transactions (
    id INTEGER NOT NULL DEFAULT nextval('transactions_id_seq'),
    account_id INTEGER NOT NULL,
    type TEXT NOT NULL,
    amount INTEGER NOT NULL DEFAULT 0,
    transaction_date DATE,
    description TEXT,
    reference TEXT,
    transfer_account_id INTEGER,
    contact_id INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS transactions;
DROP SEQUENCE IF EXISTS transactions_id_seq;
