package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"
)

// pqConnValue returns s as a safely quoted lib/pq keyword/value connection-string value.
// Values are wrapped in single quotes and single quotes within the value are doubled
// (PostgreSQL libpq escaping convention), and backslashes are escaped with a backslash.
func pqConnValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `''`)
	return "'" + s + "'"
}

// OpenWithCredentials opens a single-connection PortalDB using the given
// tenant_id as the PostgreSQL username and the JWT token as the password.
// or service account credentials (username, password)
// It reads NEXUS_HOST (default "localhost"), NEXUS_PORT (default "5433"),
// NEXUS_DATABASE (default "lake"), and NEXUS_SCHEMA (default: same as NEXUS_DATABASE)
// from the environment.
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
	schema := os.Getenv("NEXUS_SCHEMA")
	if schema == "" {
		schema = database
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, tenantID, token, database)
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open per-request database connection: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	// Verify the connection is usable before returning.  Without an upfront
	// Ping the first user (goose provider init) would receive a confusing
	// "bad connection / connection is already closed" if the Nexus gateway
	// dropped the TCP connection right after service-account rotation.
	// Retry a few times to tolerate brief propagation delays.
	var pingErr error
	for i := range 3 {
		if pingErr = sqlDB.Ping(); pingErr == nil {
			break
		}
		if i < 2 {
			slog.Warn("database ping failed, retrying", "attempt", i+1, "error", pingErr)
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
		}
	}
	if pingErr != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to connect to tenant database: %w", pingErr)
	}

	// Best-effort: set search_path via SQL so that any unqualified table
	// references resolve to the correct schema. This is belt-and-suspenders —
	// PortalDB.rebind() already prepends the schema to every table name, so
	// queries continue to work even if the Nexus gateway does not propagate
	// SET commands. pq.QuoteIdentifier safely escapes the schema name.
	if _, err := sqlDB.Exec("SET search_path TO " + pq.QuoteIdentifier(schema)); err != nil {
		slog.Warn("could not set search_path; queries will rely on schema-qualified table names",
			"schema", schema, "error", err)
	}

	return WrapDB(sqlDB), nil
}

// Open creates and returns a PortalDB connection to the Nexus gateway.
// The connection DSN is read from the DATABASE_URL environment variable.
// If DATABASE_URL is not set, individual NEXUS_HOST, NEXUS_PORT, NEXUS_USER,
// NEXUS_PASSWORD, NEXUS_DATABASE, and NEXUS_SCHEMA variables are used, defaulting
// to a local Nexus instance on port 5433. NEXUS_SCHEMA defaults to NEXUS_DATABASE.
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
		schema := os.Getenv("NEXUS_SCHEMA")
		if schema == "" {
			schema = database
		}
		// search_path is set via a quoted DSN value so that schema names
		// containing spaces or special characters are handled correctly.
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s search_path=%s sslmode=disable",
			host, port, user, password, database, pqConnValue(schema))
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to nexus gateway: %w", err)
	}

	slog.Info("connected to nexus gateway")
	return WrapDB(sqlDB), nil
}

