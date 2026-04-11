package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// OpenWithCredentials opens a single-connection PortalDB using the given
// tenant_id as the PostgreSQL username and the JWT token as the password.
// or service account credentials (username, password)
// It reads NEXUS_HOST (default "localhost"), NEXUS_PORT (default "5433"),
// and NEXUS_DATABASE (default "lake") from the environment.
// The connection is pinged (with retries) before returning to ensure it is usable.
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
