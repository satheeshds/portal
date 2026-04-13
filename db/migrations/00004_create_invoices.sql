-- +goose Up
CREATE SEQUENCE IF NOT EXISTS invoices_id_seq START 1;
CREATE TABLE IF NOT EXISTS invoices (
    id INTEGER NOT NULL DEFAULT nextval('invoices_id_seq'),
    contact_id INTEGER,
    invoice_number TEXT,
    issue_date DATE,
    due_date DATE,
    amount INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'draft',
    file_url TEXT,
    notes TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS invoices;
DROP SEQUENCE IF EXISTS invoices_id_seq;
