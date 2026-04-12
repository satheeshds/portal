package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

var tableRegex = regexp.MustCompile(`(?i)\b(FROM|JOIN|INTO|UPDATE)\s+([a-zA-Z0-9_\.]+)\b`)

// rebind converts ? positional placeholders to $N placeholders required by the
// PostgreSQL wire protocol used by the Nexus gateway. It correctly skips ? characters
// that appear inside single-quoted SQL string literals.
// Also adds 'lake.' namespace before table names if not already present.
func rebind(query string) string {
	n := 0
	var b strings.Builder
	b.Grow(len(query) + 20)
	runes := []rune(query)
	inString := false
	lastPart := 0

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\'' {
			if inString {
				// Single quote inside string
				if i+1 < len(runes) && runes[i+1] == '\'' {
					i++
					continue
				}
				// End of string literal
				b.WriteString(string(runes[lastPart : i+1]))
				lastPart = i + 1
				inString = false
			} else {
				// Entering string literal. Process leading code.
				code := string(runes[lastPart:i])
				b.WriteString(processCode(code, &n))
				lastPart = i
				inString = true
			}
		}
	}

	// Process final block
	if inString {
		b.WriteString(string(runes[lastPart:]))
	} else {
		b.WriteString(processCode(string(runes[lastPart:]), &n))
	}

	return b.String()
}

func processCode(code string, n *int) string {
	// 1. Add 'lake.' namespace to table references
	code = tableRegex.ReplaceAllStringFunc(code, func(m string) string {
		parts := tableRegex.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		keyword := parts[1]
		table := parts[2]
		if !strings.Contains(table, ".") {
			return keyword + " lake." + table
		}
		return m
	})

	// 2. Replace ? with $N
	var res strings.Builder
	for _, r := range code {
		if r == '?' {
			*n += 1
			res.WriteString(fmt.Sprintf("$%d", *n))
		} else {
			res.WriteRune(r)
		}
	}
	return res.String()
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
