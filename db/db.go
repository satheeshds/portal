package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// openDB parses the DSN, configures pgx to use QueryExecModeExec (extended
// query protocol without a Describe round-trip), and returns a *sql.DB.
//
// QueryExecModeExec sends Parse→Bind→Execute→Sync for every query.  This
// avoids two problems that arise with the Nexus gateway:
//   - No Describe step means no binary-format OID negotiation, eliminating
//     the binary type-mismatch errors seen with the default cached-statement
//     mode.
//   - No simple-query protocol means pgx never checks for
//     standard_conforming_strings in the server's ParameterStatus map, which
//     the gateway does not advertise.
func openDB(dsn string) (*sql.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeExec
	return stdlib.OpenDB(*config), nil
}

// OpenWithCredentials opens a single-connection PortalDB using the given
// tenant_id as the PostgreSQL username and the JWT token as the password.
// or service account credentials (username, password)
// It reads NEXUS_HOST (default "localhost"), NEXUS_PORT (default "5433"),
// and NEXUS_DATABASE (default "lake") from the environment.
// The connection is not pinged; the first query will surface any auth errors.
func OpenWithCredentials(ctx context.Context, tenantID, token string) (*PortalDB, error) {
	host := os.Getenv("NEXUS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("NEXUS_PORT")
	if port == "" {
		port = "5433"
	}
	database := os.Getenv("NEXUS_DATABASE")
	if database == "" {
		database = "lake"
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, tenantID, token, database)
	sqlDB, err := openDB(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open per-request database connection: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return WrapDB(ctx, sqlDB), nil
}
