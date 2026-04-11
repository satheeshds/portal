-- +goose Up
CREATE TABLE IF NOT EXISTS recurring_payments (
    id INTEGER NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    amount INTEGER NOT NULL,
    account_id INTEGER NOT NULL,
    contact_id INTEGER,
    frequency TEXT NOT NULL,
    interval INTEGER NOT NULL DEFAULT 1,
    start_date DATE NOT NULL,
    end_date DATE,
    next_due_date DATE,
    last_generated_date DATE,
    status TEXT NOT NULL DEFAULT 'active',
    description TEXT,
    reference TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS recurring_payments;
