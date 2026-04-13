package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// nexusHTTPClient is a shared HTTP client used for all nexus-control admin API calls.
var nexusHTTPClient = &http.Client{Timeout: 30 * time.Second}

type tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type serviceAccount struct {
	Username string `json:"service_id"`
	Password string `json:"service_api_key"`
}

// MigrateAllTenants lists every tenant from nexus-control, connects to each
// tenant's database via the nexus gateway using rotated service-account credentials, and
// runs schema migrations for each one (no occurrence generation).
//
// This function is intended to be called once on startup by the platform service.
// After migrations, GenerateOccurrencesForAllTenants should be called separately.
func MigrateAllTenants(controlURL, adminKey string) error {
	return forEachTenant(controlURL, adminKey, "migrating schema", MigrateTenant)
}

// GenerateOccurrencesForAllTenants lists every tenant from nexus-control, connects to each
// tenant's database, and runs one-shot occurrence generation for each one. It does not
// re-run migrations. Intended for the daily scheduled run by the platform service.
func GenerateOccurrencesForAllTenants(controlURL, adminKey string) error {
	return forEachTenant(controlURL, adminKey, "generating occurrences", func(portalDB *PortalDB, tenantID string) error {
		return GenerateRecurringOccurrences(portalDB)
	})
}

// forEachTenant lists all tenants, rotates credentials, opens a DB connection for each,
// and calls fn(db, tenantID). Errors from fn are logged but do not abort the loop.
func forEachTenant(controlURL, adminKey, action string, fn func(*PortalDB, string) error) error {
	tenants, err := listAllTenants(controlURL, adminKey)
	if err != nil {
		return fmt.Errorf("failed to list tenants: %w", err)
	}

	slog.Info("processing tenants", "action", action, "count", len(tenants))

	for _, t := range tenants {
		slog.Info(action+" for tenant", "tenant_id", t.ID, "tenant_name", t.Name)

		creds, err := RotateTenantServiceAccount(controlURL, adminKey, t.ID)
		if err != nil {
			slog.Error("failed to rotate service account", "tenant_id", t.ID, "error", err)
			continue
		}

		portalDB, err := OpenWithCredentials(creds.Username, creds.Password)
		if err != nil {
			slog.Error("failed to open database connection", "tenant_id", t.ID, "error", err)
			continue
		}

		if err := fn(portalDB, t.ID); err != nil {
			slog.Error("failed to "+action, "tenant_id", t.ID, "error", err)
		}
		portalDB.Close()
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

	resp, err := nexusHTTPClient.Do(req)
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

func RotateTenantServiceAccount(controlURL, adminKey, tenantID string) (*serviceAccount, error) {
	endpoint := fmt.Sprintf("%s/api/v1/admin/tenants/%s/service-account/rotate", controlURL, tenantID)

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-API-Key", adminKey)

	resp, err := nexusHTTPClient.Do(req)
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
	if sa.Username == "" || sa.Password == "" {
		return nil, fmt.Errorf("rotate service account response missing required credentials for tenant %s", tenantID)
	}

	slog.Debug("rotated service account", "tenant_id", tenantID, "service_account", sa)

	return &sa, nil
}
