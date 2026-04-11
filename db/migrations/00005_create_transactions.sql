-- +goose Up
CREATE TABLE IF NOT EXISTS transactions (
    id INTEGER NOT NULL,
    account_id INTEGER NOT NULL,
    type TEXT NOT NULL,
    amount INTEGER NOT NULL DEFAULT 0,
    transaction_date DATE,
    description TEXT,
    reference TEXT,
    transfer_account_id INTEGER,
    contact_id INTEGER,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS transactions;
