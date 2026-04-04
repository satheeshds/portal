package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/lib/pq"
)

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
