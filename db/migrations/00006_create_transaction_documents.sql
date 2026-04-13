-- +goose Up
-- Junction table: many-to-many transaction <-> bill/invoice/payout/recurring_payment_occurrence.
-- Valid document_type values are enforced by the application layer.
CREATE SEQUENCE IF NOT EXISTS transaction_documents_id_seq START 1;
CREATE TABLE IF NOT EXISTS transaction_documents (
    id INTEGER NOT NULL DEFAULT nextval('transaction_documents_id_seq'),
    transaction_id INTEGER NOT NULL,
    document_type TEXT NOT NULL,
    document_id INTEGER NOT NULL,
    amount INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS transaction_documents;
DROP SEQUENCE IF EXISTS transaction_documents_id_seq;
