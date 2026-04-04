package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// rebind converts ? positional placeholders to $N placeholders required by the
// PostgreSQL wire protocol used by the Nexus gateway. It correctly skips ? characters
// that appear inside single-quoted SQL string literals.
func rebind(query string) string {
	if !strings.Contains(query, "?") {
		return query
	}
	n := 0
	var b strings.Builder
	b.Grow(len(query) + 10)
	runes := []rune(query)
	inString := false
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if inString {
			b.WriteRune(r)
			if r == '\'' {
				// A doubled single-quote '' is an escaped quote inside a string literal.
				if i+1 < len(runes) && runes[i+1] == '\'' {
					b.WriteRune(runes[i+1])
					i++
				} else {
					inString = false
				}
			}
		} else {
			switch r {
			case '\'':
				inString = true
				b.WriteRune(r)
			case '?':
				n++
				b.WriteString(fmt.Sprintf("$%d", n))
			default:
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// PortalDB wraps *sql.DB and automatically rebinds ? placeholders to $N
// for compatibility with the PostgreSQL wire protocol used by the Nexus gateway.
type PortalDB struct {
	*sql.DB
}

// WrapDB wraps an existing *sql.DB in PortalDB. Useful for testing.
func WrapDB(db *sql.DB) *PortalDB {
	return &PortalDB{db}
}

// Query rebinds ? placeholders before executing the query.
func (d *PortalDB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.DB.Query(rebind(query), args...)
}

// QueryRow rebinds ? placeholders before executing the query.
func (d *PortalDB) QueryRow(query string, args ...any) *sql.Row {
	return d.DB.QueryRow(rebind(query), args...)
}

// Exec rebinds ? placeholders before executing the statement.
func (d *PortalDB) Exec(query string, args ...any) (sql.Result, error) {
	return d.DB.Exec(rebind(query), args...)
}

// Prepare rebinds ? placeholders before preparing the statement.
func (d *PortalDB) Prepare(query string) (*sql.Stmt, error) {
	return d.DB.Prepare(rebind(query))
}

// Begin starts a transaction and returns a PortalTx that also auto-rebinds.
func (d *PortalDB) Begin() (*PortalTx, error) {
	tx, err := d.DB.Begin()
	if err != nil {
		return nil, err
	}
	return &PortalTx{tx}, nil
}

// BeginTx starts a transaction with context and options, and returns a PortalTx
// that auto-rebinds ? placeholders to $N.
func (d *PortalDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*PortalTx, error) {
	tx, err := d.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &PortalTx{tx}, nil
}

// PortalTx wraps *sql.Tx and automatically rebinds ? placeholders to $N.
type PortalTx struct {
	*sql.Tx
}

// Query rebinds ? placeholders before executing the query.
func (t *PortalTx) Query(query string, args ...any) (*sql.Rows, error) {
	return t.Tx.Query(rebind(query), args...)
}

// QueryRow rebinds ? placeholders before executing the query.
func (t *PortalTx) QueryRow(query string, args ...any) *sql.Row {
	return t.Tx.QueryRow(rebind(query), args...)
}

// Exec rebinds ? placeholders before executing the statement.
func (t *PortalTx) Exec(query string, args ...any) (sql.Result, error) {
	return t.Tx.Exec(rebind(query), args...)
}

// Prepare rebinds ? placeholders before preparing the statement.
func (t *PortalTx) Prepare(query string) (*sql.Stmt, error) {
	return t.Tx.Prepare(rebind(query))
}
