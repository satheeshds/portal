-- +goose Up
CREATE SEQUENCE IF NOT EXISTS invoice_items_id_seq START 1;
CREATE TABLE IF NOT EXISTS invoice_items (
    id INTEGER NOT NULL DEFAULT nextval('invoice_items_id_seq'),
    invoice_id INTEGER NOT NULL,
    description TEXT NOT NULL,
    quantity DOUBLE NOT NULL DEFAULT 1,
    unit TEXT,
    unit_price INTEGER NOT NULL DEFAULT 0,
    amount INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS invoice_items;
DROP SEQUENCE IF EXISTS invoice_items_id_seq;
