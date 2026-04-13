package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/satheeshds/portal/db"
)

func main() {
	// Configure structured logging
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	controlURL, adminKey, err := platformConfig()
	if err != nil {
		slog.Error("platform configuration error", "error", err)
		os.Exit(1)
	}

	// Migrate schemas once at startup for all tenants.
	if err := db.MigrateAllTenants(controlURL, adminKey); err != nil {
		slog.Warn("schema migration failed on startup", "error", err)
	}

	// Generate occurrences once at startup (gap recovery for any missed periods).
	if err := db.GenerateOccurrencesForAllTenants(controlURL, adminKey); err != nil {
		slog.Warn("occurrence generation failed on startup", "error", err)
	}

	// Daily loop: re-run only occurrence generation (migrations already applied above).
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
		slog.Info("next occurrence generation scheduled", "at", next.Format(time.RFC3339))
		time.Sleep(time.Until(next))

		if err := db.GenerateOccurrencesForAllTenants(controlURL, adminKey); err != nil {
			slog.Warn("daily occurrence generation failed", "error", err)
		}
	}
}

// platformConfig reads and validates the environment variables required by the platform service.
func platformConfig() (controlURL, adminKey string, err error) {
	controlURL = os.Getenv("NEXUS_CONTROL_URL")
	if controlURL == "" {
		controlURL = "http://nexus-control:8080"
	}
	adminKey = os.Getenv("ADMIN_API_KEY")
	if adminKey == "" {
		return "", "", fmt.Errorf("ADMIN_API_KEY is required")
	}
	return controlURL, adminKey, nil
}
