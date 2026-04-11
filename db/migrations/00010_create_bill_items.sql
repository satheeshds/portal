-- +goose Up
CREATE TABLE IF NOT EXISTS bill_items (
    id INTEGER NOT NULL,
    bill_id INTEGER NOT NULL,
    description TEXT NOT NULL,
    quantity DOUBLE NOT NULL DEFAULT 1,
    unit TEXT,
    unit_price INTEGER NOT NULL DEFAULT 0,
    amount INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS bill_items;
