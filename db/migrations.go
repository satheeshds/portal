package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func init() {
	// Silence goose's default logger; portal uses slog.
	goose.SetLogger(goose.NopLogger())
}

// MigrateDB applies all pending up-migrations to the provided database.
// It is idempotent: already-applied migrations are skipped.
// Goose records applied migrations in the goose_db_version tracking table.
//
// This is intended for use in tests where a live Nexus control endpoint is
// not available (e.g. an in-process DuckDB instance), as well as for
// single-tenant migration during registration.
func MigrateDB(db *PortalDB) error {
	slog.Info("running database migrations via db connection")

	provider, err := newProvider(db)
	if err != nil {
		return err
	}

	results, err := provider.Up(context.Background())
	if err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}

	for _, r := range results {
		slog.Info("applied migration", "version", r.Source.Version, "duration", r.Duration)
	}

	slog.Info("database migrations complete")
	return nil
}

// RollbackDB rolls back the last n applied migrations in reverse order.
// Pass n <= 0 to roll back all applied migrations.
func RollbackDB(db *PortalDB, n int) error {
	provider, err := newProvider(db)
	if err != nil {
		return err
	}

	if n <= 0 {
		// Roll back everything down to version 0.
		results, err := provider.DownTo(context.Background(), 0)
		if err != nil {
			return fmt.Errorf("database rollback failed: %w", err)
		}
		for _, r := range results {
			slog.Info("rolled back migration", "version", r.Source.Version, "duration", r.Duration)
		}
		slog.Info("rollback complete", "rolled_back", len(results))
		return nil
	}

	// Roll back n steps one at a time.
	for i := 0; i < n; i++ {
		result, err := provider.Down(context.Background())
		if err != nil {
			return fmt.Errorf("database rollback step %d failed: %w", i+1, err)
		}
		if result != nil {
			slog.Info("rolled back migration", "version", result.Source.Version, "duration", result.Duration)
		}
	}

	slog.Info("rollback complete", "rolled_back", n)
	return nil
}

// MigrateTenant runs schema migrations for a single tenant database.
// Occurrence generation is handled separately by the platform service.
func MigrateTenant(tenantDB *PortalDB, tenantID string) error {
	slog.Info("migrating tenant schema", "tenant_id", tenantID)

	if err := MigrateDB(tenantDB); err != nil {
		return fmt.Errorf("migration failed for tenant %s: %w", tenantID, err)
	}

	slog.Info("migration complete for tenant", "tenant_id", tenantID)
	return nil
}

// newProvider builds a goose provider backed by the embedded SQL migration
// files and the raw *sql.DB (bypassing the lake. schema prefix rewrite used
// by PortalDB for application queries).
func newProvider(db *PortalDB) (*goose.Provider, error) {
	migFS, err := fs.Sub(embedMigrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to create migration filesystem: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db.DB, migFS)
	if err != nil {
		return nil, fmt.Errorf("failed to create goose provider: %w", err)
	}
	return provider, nil
}
