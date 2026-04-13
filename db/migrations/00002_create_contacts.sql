-- +goose Up
CREATE SEQUENCE IF NOT EXISTS contacts_id_seq START 1;
CREATE TABLE IF NOT EXISTS contacts (
    id INTEGER NOT NULL DEFAULT nextval('contacts_id_seq'),
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS contacts;
DROP SEQUENCE IF EXISTS contacts_id_seq;
