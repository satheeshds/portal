package db

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	_ "github.com/lib/pq"
)

type tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type serviceAccount struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
}

// MigrateAndGenerateAllTenants lists every tenant from nexus-control, connects to each
// tenant's database via the nexus gateway using rotated service-account credentials, and
// calls MigrateAndGenerateTenant for each one.
//
// This function is intended to be called on startup (gap recovery) and on the daily schedule
// by the platform service. It is also safe to call in any context that needs to process all
// tenants in a single pass.
func MigrateAndGenerateAllTenants(controlURL, adminKey, nexusHost, nexusPort string) error {
	tenants, err := listAllTenants(controlURL, adminKey)
	if err != nil {
		return fmt.Errorf("failed to list tenants: %w", err)
	}

	slog.Info("processing migration and occurrence generation for tenants", "count", len(tenants))

	for _, t := range tenants {
		slog.Info("migrating and generating occurrences for tenant", "tenant_id", t.ID, "tenant_name", t.Name)

		creds, err := rotateTenantServiceAccount(controlURL, adminKey, t.ID)
		if err != nil {
			slog.Error("failed to rotate service account", "tenant_id", t.ID, "error", err)
			continue
		}

		connURL := &url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword(creds.Username, creds.Password),
			Host:     nexusHost + ":" + nexusPort,
			Path:     "/" + creds.Database,
			RawQuery: "sslmode=disable",
		}

		sqlDB, err := sql.Open("postgres", connURL.String())
		if err != nil {
			slog.Error("failed to open database connection", "tenant_id", t.ID, "error", err)
			continue
		}

		database := WrapDB(sqlDB)
		if err := MigrateAndGenerateTenant(database, t.ID); err != nil {
			slog.Error("failed to migrate and generate occurrences", "tenant_id", t.ID, "error", err)
		}
		database.Close()
	}

	return nil
}

func listAllTenants(controlURL, adminKey string) ([]tenant, error) {
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
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("list tenants returned status %d (could not read body: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("list tenants returned status %d: %s", resp.StatusCode, string(body))
	}

	var tenants []tenant
	if err := json.NewDecoder(resp.Body).Decode(&tenants); err != nil {
		return nil, fmt.Errorf("failed to decode tenants response: %w", err)
	}

	return tenants, nil
}

func rotateTenantServiceAccount(controlURL, adminKey, tenantID string) (*serviceAccount, error) {
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
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("rotate service account returned status %d (could not read body: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("rotate service account returned status %d: %s", resp.StatusCode, string(body))
	}

	var sa serviceAccount
	if err := json.NewDecoder(resp.Body).Decode(&sa); err != nil {
		return nil, fmt.Errorf("failed to decode service account response: %w", err)
	}

	return &sa, nil
}
