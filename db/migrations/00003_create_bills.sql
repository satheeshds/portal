-- +goose Up
CREATE TABLE IF NOT EXISTS bills (
    id INTEGER NOT NULL,
    contact_id INTEGER,
    bill_number TEXT,
    issue_date DATE,
    due_date DATE,
    amount INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'draft',
    file_url TEXT,
    notes TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS bills;
