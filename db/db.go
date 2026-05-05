package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
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

	// Register a lenient date codec on every new connection so that Nexus
	// gateway DATE columns sent as timestamp-like strings (e.g. "2026-04-27
	// 00:00:00") are accepted in addition to the strict YYYY-MM-DD format that
	// the default pgtype.DateCodec requires.
	afterConnect := stdlib.OptionAfterConnect(func(_ context.Context, conn *pgx.Conn) error {
		registerLenientDateCodec(conn.TypeMap())
		return nil
	})

	return stdlib.OpenDB(*config, afterConnect), nil
}

// OpenWithCredentials opens a single-connection PortalDB using the given
// tenant_id as the PostgreSQL username and the JWT token as the password.
// or service account credentials (username, password)
// It reads NEXUS_HOST (default "localhost"), NEXUS_PORT (default "5433"),
// and NEXUS_DATABASE (default "lake") from the environment.
// The connection is not pinged; the first query will surface any auth errors.
func OpenWithCredentials(tenantID, token string) (*PortalDB, error) {
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
	return WrapDB(sqlDB), nil
}

// Open creates and returns a PortalDB connection to the Nexus gateway.
// The connection DSN is read from the DATABASE_URL environment variable.
// If DATABASE_URL is not set, individual NEXUS_HOST, NEXUS_PORT, NEXUS_USER,
// NEXUS_PASSWORD, and NEXUS_DATABASE variables are used, defaulting to a
// local Nexus instance on port 5433.
func Open() (*PortalDB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		host := os.Getenv("NEXUS_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("NEXUS_PORT")
		if port == "" {
			port = "5433"
		}
		user := os.Getenv("NEXUS_USER")
		if user == "" {
			user = "portal"
		}
		password := os.Getenv("NEXUS_PASSWORD")
		database := os.Getenv("NEXUS_DATABASE")
		if database == "" {
			database = "lake"
		}
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			host, port, user, password, database)
	}

	sqlDB, err := openDB(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to nexus gateway: %w", err)
	}

	slog.Info("connected to nexus gateway")
	return WrapDB(sqlDB), nil
}

