-- +goose Up
CREATE TABLE IF NOT EXISTS contacts (
    id INTEGER NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS contacts;
