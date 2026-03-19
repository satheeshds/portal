package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/duckdb/duckdb-go/v2"
)

const duckLakeCatalog = "accounting"

type connectionConfig struct {
	metadataPath    string
	dataPath        string
	bootstrapDBPath string
	preferDuckLake  bool
	requireDuckLake bool
}

type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// Open creates and returns the application's database connection.
// It prefers opening a DuckLake catalog and falls back to a plain DuckDB database
// when the DuckLake extension is unavailable unless DUCKLAKE_REQUIRED=true.
func Open() (*sql.DB, error) {
	cfg := loadConnectionConfig()

	if cfg.preferDuckLake {
		db, err := openDuckLake(cfg)
		if err == nil {
			return db, nil
		}
		if cfg.requireDuckLake {
			return nil, err
		}

		slog.Warn("ducklake unavailable, falling back to duckdb",
			"metadata_path", cfg.metadataPath,
			"data_path", cfg.dataPath,
			"bootstrap_path", cfg.bootstrapDBPath,
			"error", err,
		)
	}

	db, err := openDuckDB(cfg.metadataPath, false)
	if err != nil {
		return nil, err
	}
	slog.Info("connected to duckdb", "path", cfg.metadataPath)
	return db, nil
}

func openDuckLake(cfg connectionConfig) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.metadataPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create ducklake metadata directory: %w", err)
	}
	if err := os.MkdirAll(cfg.dataPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create ducklake data directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.bootstrapDBPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create duckdb bootstrap directory: %w", err)
	}

	db, err := openDuckDB(cfg.bootstrapDBPath, true)
	if err != nil {
		return nil, err
	}

	if err := configureDuckLake(db, cfg); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize ducklake: %w", err)
	}

	slog.Info("connected to ducklake",
		"metadata_path", cfg.metadataPath,
		"data_path", cfg.dataPath,
		"bootstrap_path", cfg.bootstrapDBPath,
	)
	return db, nil
}

func openDuckDB(dbPath string, singleConnection bool) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if singleConnection {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func configureDuckLake(exec sqlExecer, cfg connectionConfig) error {
	for _, stmt := range duckLakeStatements(cfg) {
		if _, err := exec.Exec(stmt); err != nil {
			return fmt.Errorf("statement %q failed: %w", stmt, err)
		}
	}
	return nil
}

func duckLakeStatements(cfg connectionConfig) []string {
	return []string{
		"INSTALL ducklake",
		"LOAD ducklake",
		fmt.Sprintf(
			"ATTACH 'ducklake:%s' AS %s (DATA_PATH '%s')",
			escapeDuckLakePath(cfg.metadataPath),
			duckLakeCatalog,
			escapeDuckLakePath(cfg.dataPath),
		),
		fmt.Sprintf("USE %s", duckLakeCatalog),
	}
}

func loadConnectionConfig() connectionConfig {
	metadataPath := os.Getenv("DB_PATH")
	if metadataPath == "" {
		metadataPath = "./data/accounting.ducklake"
	}

	dataPath := os.Getenv("DUCKLAKE_DATA_PATH")
	if dataPath == "" {
		dataPath = defaultDuckLakeDataPath(metadataPath)
	}

	bootstrapDBPath := os.Getenv("DUCKLAKE_BOOTSTRAP_PATH")
	if bootstrapDBPath == "" {
		bootstrapDBPath = defaultBootstrapDBPath(metadataPath)
	}

	return connectionConfig{
		metadataPath:    metadataPath,
		dataPath:        dataPath,
		bootstrapDBPath: bootstrapDBPath,
		preferDuckLake:  envBool("DUCKLAKE_ENABLED", true),
		requireDuckLake: envBool("DUCKLAKE_REQUIRED", false),
	}
}

func defaultDuckLakeDataPath(metadataPath string) string {
	return filepath.Join(filepath.Dir(metadataPath), metadataBaseName(metadataPath))
}

func defaultBootstrapDBPath(metadataPath string) string {
	return filepath.Join(filepath.Dir(metadataPath), metadataBaseName(metadataPath)+".bootstrap.duckdb")
}

func metadataBaseName(metadataPath string) string {
	base := filepath.Base(metadataPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name == "" {
		return base
	}
	return name
}

func escapeDuckLakePath(path string) string {
	return strings.ReplaceAll(path, "'", "''")
}

func envBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}
