package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/pressly/goose/v3"
)

// totalMigrations is the number of SQL migration files in db/migrations/.
const totalMigrations = 11

// allApplicationTables lists every application table created by the migrations,
// in the order they are created (one per SQL file).
var allApplicationTables = []string{
	"accounts",
	"contacts",
	"bills",
	"invoices",
	"transactions",
	"transaction_documents",
	"payouts",
	"recurring_payments",
	"recurring_payment_occurrences",
	"bill_items",
	"invoice_items",
}

// openTestDB opens an in-file DuckDB database suitable for migration tests.
func openTestDB(t *testing.T) *PortalDB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, fmt.Sprintf("test_migrations_%s.db", t.Name()))
	rawDB, err := sql.Open("duckdb", dbPath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { rawDB.Close() })
	return WrapDB(rawDB)
}

// migrateTestDB applies all pending migrations to a test (DuckDB) database using
// the DuckDB-compatible provider. Tests must use this instead of MigrateDB.
func migrateTestDB(t *testing.T, db *PortalDB) {
	t.Helper()
	provider, err := newTestProvider(db)
	if err != nil {
		t.Fatalf("newTestProvider: %v", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		t.Fatalf("provider.Up: %v", err)
	}
}

// rollbackTestDB rolls back the last n migrations from a test (DuckDB) database.
// Pass n <= 0 to roll back all.
func rollbackTestDB(t *testing.T, db *PortalDB, n int) {
	t.Helper()
	provider, err := newTestProvider(db)
	if err != nil {
		t.Fatalf("newTestProvider: %v", err)
	}
	if n <= 0 {
		if _, err := provider.DownTo(context.Background(), 0); err != nil {
			t.Fatalf("provider.DownTo(0): %v", err)
		}
		return
	}
	for i := 0; i < n; i++ {
		if _, err := provider.Down(context.Background()); err != nil {
			t.Fatalf("provider.Down step %d: %v", i+1, err)
		}
	}
}

// countApplied returns the number of migrations in the "applied" state via the
// DuckDB-compatible goose provider.
func countApplied(t *testing.T, db *PortalDB) int {
	t.Helper()
	provider, err := newTestProvider(db)
	if err != nil {
		t.Fatalf("newTestProvider: %v", err)
	}
	statuses, err := provider.Status(context.Background())
	if err != nil {
		t.Fatalf("provider.Status: %v", err)
	}
	n := 0
	for _, s := range statuses {
		if s.State == goose.StateApplied {
			n++
		}
	}
	return n
}

// tableExists reports whether the named table exists in the database.
func tableExists(t *testing.T, db *PortalDB, name string) bool {
	t.Helper()
	row := db.DB.QueryRow(
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = $1`, name)
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("check table existence %q: %v", name, err)
	}
	return n > 0
}

// TestMigrateDB_AppliesAllMigrations verifies that migrations create all
// expected application tables and that goose records each one as applied.
func TestMigrateDB_AppliesAllMigrations(t *testing.T) {
	db := openTestDB(t)
	migrateTestDB(t, db)

	if got := countApplied(t, db); got != totalMigrations {
		t.Errorf("applied migrations = %d, want %d", got, totalMigrations)
	}

	for _, tbl := range allApplicationTables {
		if !tableExists(t, db, tbl) {
			t.Errorf("expected table %q to exist after migration", tbl)
		}
	}
}

// TestMigrateDB_Idempotent verifies that calling migrate a second time does not
// apply migrations again or return an error.
func TestMigrateDB_Idempotent(t *testing.T) {
	db := openTestDB(t)
	migrateTestDB(t, db)
	firstCount := countApplied(t, db)

	migrateTestDB(t, db)
	secondCount := countApplied(t, db)

	if firstCount != secondCount {
		t.Errorf("applied count changed on second run: %d → %d", firstCount, secondCount)
	}
}

// TestRollbackDB_LastN verifies that rolling back n migrations removes exactly
// those tables.
func TestRollbackDB_LastN(t *testing.T) {
	db := openTestDB(t)
	migrateTestDB(t, db)
	total := countApplied(t, db)

	const rollbackN = 3
	rollbackTestDB(t, db, rollbackN)

	if got, want := countApplied(t, db), total-rollbackN; got != want {
		t.Errorf("after rollback %d: applied = %d, want %d", rollbackN, got, want)
	}

	// Tables for the last rollbackN migrations must no longer exist.
	for _, tbl := range allApplicationTables[len(allApplicationTables)-rollbackN:] {
		if tableExists(t, db, tbl) {
			t.Errorf("table %q should not exist after rolling back last %d migrations", tbl, rollbackN)
		}
	}
}

// TestRollbackDB_All verifies that rolling back everything (n=0) removes all
// application tables.
func TestRollbackDB_All(t *testing.T) {
	db := openTestDB(t)
	migrateTestDB(t, db)
	rollbackTestDB(t, db, 0)

	if got := countApplied(t, db); got != 0 {
		t.Errorf("expected 0 applied migrations after full rollback, got %d", got)
	}

	for _, tbl := range allApplicationTables {
		if tableExists(t, db, tbl) {
			t.Errorf("table %q should not exist after full rollback", tbl)
		}
	}
}

// TestRollbackDB_ThenReapply verifies that after a full rollback the migrations
// can be re-applied cleanly.
func TestRollbackDB_ThenReapply(t *testing.T) {
	db := openTestDB(t)
	migrateTestDB(t, db)
	rollbackTestDB(t, db, 0)
	migrateTestDB(t, db)

	if got := countApplied(t, db); got != totalMigrations {
		t.Errorf("re-applied migrations count = %d, want %d", got, totalMigrations)
	}
}

// TestMigrateDB_VersionsAreCorrect verifies that every migration is recorded
// with the expected version number (1–11) after a full apply.
func TestMigrateDB_VersionsAreCorrect(t *testing.T) {
	db := openTestDB(t)
	migrateTestDB(t, db)

	provider, err := newTestProvider(db)
	if err != nil {
		t.Fatalf("newTestProvider: %v", err)
	}
	statuses, err := provider.Status(context.Background())
	if err != nil {
		t.Fatalf("provider.Status: %v", err)
	}

	if len(statuses) != totalMigrations {
		t.Fatalf("expected %d migration statuses, got %d", totalMigrations, len(statuses))
	}

	for i, s := range statuses {
		if s.State != goose.StateApplied {
			t.Errorf("migration[%d] version=%d: expected state %q, got %q",
				i, s.Source.Version, goose.StateApplied, s.State)
		}
		wantVersion := int64(i + 1)
		if s.Source.Version != wantVersion {
			t.Errorf("migration[%d] version = %d, want %d", i, s.Source.Version, wantVersion)
		}
	}
}
