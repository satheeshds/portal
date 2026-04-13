package store

import "github.com/satheeshds/portal/db"

// Store is the data access layer that wraps a database connection.
type Store struct {
	db *db.PortalDB
}

// New creates a new Store backed by the given database connection.
func New(d *db.PortalDB) *Store {
	return &Store{db: d}
}
