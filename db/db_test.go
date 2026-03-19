package db

import (
	"strings"
	"testing"
)

func TestLoadConnectionConfigDefaults(t *testing.T) {
	t.Setenv("DB_PATH", "")
	t.Setenv("DUCKLAKE_DATA_PATH", "")
	t.Setenv("DUCKLAKE_BOOTSTRAP_PATH", "")
	t.Setenv("DUCKLAKE_ENABLED", "")
	t.Setenv("DUCKLAKE_REQUIRED", "")

	cfg := loadConnectionConfig()

	if cfg.metadataPath != "./data/accounting.ducklake" {
		t.Fatalf("metadata path = %q, want %q", cfg.metadataPath, "./data/accounting.ducklake")
	}
	if cfg.dataPath != "data/accounting" && cfg.dataPath != "./data/accounting" {
		t.Fatalf("data path = %q, want %q", cfg.dataPath, "./data/accounting")
	}
	if cfg.bootstrapDBPath != "data/accounting.bootstrap.duckdb" && cfg.bootstrapDBPath != "./data/accounting.bootstrap.duckdb" {
		t.Fatalf("bootstrap path = %q, want %q", cfg.bootstrapDBPath, "./data/accounting.bootstrap.duckdb")
	}
	if !cfg.preferDuckLake {
		t.Fatal("preferDuckLake = false, want true")
	}
	if cfg.requireDuckLake {
		t.Fatal("requireDuckLake = true, want false")
	}
}

func TestDuckLakeStatementsEscapePaths(t *testing.T) {
	cfg := connectionConfig{
		metadataPath: "/tmp/tenant's/accounting.ducklake",
		dataPath:     "/tmp/tenant's/data",
	}

	stmts := duckLakeStatements(cfg)
	if len(stmts) != 4 {
		t.Fatalf("statement count = %d, want 4", len(stmts))
	}
	if stmts[0] != "INSTALL ducklake" {
		t.Fatalf("unexpected first statement %q", stmts[0])
	}

	attach := stmts[2]
	if !strings.Contains(attach, "ducklake:/tmp/tenant''s/accounting.ducklake") {
		t.Fatalf("attach statement did not escape metadata path: %q", attach)
	}
	if !strings.Contains(attach, "DATA_PATH '/tmp/tenant''s/data'") {
		t.Fatalf("attach statement did not escape data path: %q", attach)
	}
	if stmts[3] != "USE accounting" {
		t.Fatalf("unexpected USE statement %q", stmts[3])
	}
}
