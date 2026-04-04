package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/satheeshds/portal/db"
	_ "github.com/lib/pq"
)

type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TenantsResponse struct {
	Tenants []Tenant `json:"tenants"`
}

type ServiceAccountResponse struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
}

func main() {
	// Configure structured logging
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	// Run schema migrations via nexus-control admin API (applies to all tenants).
	if err := db.Migrate(); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Generate recurring payment occurrences immediately on startup (gap recovery),
	// then repeat daily at midnight.
	if err := generateOccurrencesForAllTenants(); err != nil {
		slog.Warn("occurrence generation failed on startup", "error", err)
	}

	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		time.Sleep(time.Until(next))
		if err := generateOccurrencesForAllTenants(); err != nil {
			slog.Warn("daily occurrence generation failed", "error", err)
		}
	}
}

func generateOccurrencesForAllTenants() error {
	controlURL := os.Getenv("NEXUS_CONTROL_URL")
	if controlURL == "" {
		controlURL = "http://nexus-control:8080"
	}
	adminKey := os.Getenv("ADMIN_API_KEY")
	if adminKey == "" {
		return fmt.Errorf("ADMIN_API_KEY is required")
	}

	nexusHost := os.Getenv("NEXUS_HOST")
	if nexusHost == "" {
		nexusHost = "nexus-gateway"
	}
	nexusPort := os.Getenv("NEXUS_PORT")
	if nexusPort == "" {
		nexusPort = "5433"
	}

	// List all tenants
	tenants, err := listTenants(controlURL, adminKey)
	if err != nil {
		return fmt.Errorf("failed to list tenants: %w", err)
	}

	slog.Info("processing occurrence generation for tenants", "count", len(tenants))

	for _, tenant := range tenants {
		slog.Info("generating occurrences for tenant", "tenant_id", tenant.ID, "tenant_name", tenant.Name)

		// Rotate service account to get credentials
		serviceAccount, err := rotateServiceAccount(controlURL, adminKey, tenant.ID)
		if err != nil {
			slog.Error("failed to rotate service account", "tenant_id", tenant.ID, "error", err)
			continue
		}

		// Connect to the nexus gateway using tenant-specific credentials
		connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			nexusHost, nexusPort, serviceAccount.Username, serviceAccount.Password, serviceAccount.Database)

		sqlDB, err := sql.Open("postgres", connStr)
		if err != nil {
			slog.Error("failed to open database connection", "tenant_id", tenant.ID, "error", err)
			continue
		}

		database := db.WrapDB(sqlDB)

		// Generate occurrences for this tenant
		if err := db.GenerateOccurrences(database); err != nil {
			slog.Error("failed to generate occurrences", "tenant_id", tenant.ID, "error", err)
		}

		database.Close()
	}

	return nil
}

func listTenants(controlURL, adminKey string) ([]Tenant, error) {
	endpoint := controlURL + "/api/v1/admin/tenants"

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Admin-API-Key", adminKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list tenants returned status %d: %s", resp.StatusCode, string(body))
	}

	var tenantsResp TenantsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tenantsResp); err != nil {
		return nil, fmt.Errorf("failed to decode tenants response: %w", err)
	}

	return tenantsResp.Tenants, nil
}

func rotateServiceAccount(controlURL, adminKey, tenantID string) (*ServiceAccountResponse, error) {
	endpoint := fmt.Sprintf("%s/api/v1/admin/tenants/%s/service-account/rotate", controlURL, tenantID)

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Admin-API-Key", adminKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rotate service account returned status %d: %s", resp.StatusCode, string(body))
	}

	var serviceAccount ServiceAccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&serviceAccount); err != nil {
		return nil, fmt.Errorf("failed to decode service account response: %w", err)
	}

	return &serviceAccount, nil
}
