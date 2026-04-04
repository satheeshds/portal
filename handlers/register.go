package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/satheeshds/portal/db"
	_ "github.com/lib/pq"
)

type registerWithMigrationRequest struct {
	OrgName  string `json:"org_name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerWithMigrationResponse struct {
	TenantID string `json:"tenant_id"`
}

// nexusRegisterWithMigrationResponse is the JSON response from the nexus registration endpoint.
type nexusRegisterWithMigrationResponse struct {
	TenantID string `json:"tenant_id"`
	Error    string `json:"error,omitempty"`
}

// serviceAccountResponse is the JSON response from the nexus service account rotation endpoint.
type serviceAccountWithMigrationResponse struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
}

// RegisterWithMigration handles POST /api/v1/register.
// It provisions a new tenant via nexus-control, connects to the tenant's database,
// and then runs portal database migrations and occurrence generation for that tenant.
//
// @Summary     Register a new tenant with migration
// @Description Provision a new tenant via nexus and initialise the portal schema for it.
// @Tags        tenants
// @Accept      json
// @Produce     json
// @Param       body body registerWithMigrationRequest true "Registration data"
// @Success     201 {object} registerWithMigrationResponse
// @Failure     400 {object} Response
// @Failure     409 {object} Response
// @Failure     500 {object} Response
// @Router      /api/v1/register [post]
func RegisterWithMigration(w http.ResponseWriter, r *http.Request) {
	var req registerWithMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OrgName == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "org_name, email, and password are required")
		return
	}

	nexusURL := nexusControlURLForMigration()
	adminKey := os.Getenv("ADMIN_API_KEY")
	if adminKey == "" {
		slog.Error("ADMIN_API_KEY is not set")
		writeError(w, http.StatusInternalServerError, "server configuration error")
		return
	}

	// Step 1: Create the tenant in nexus-control.
	tenantID, statusCode, err := createTenantViaNexusForMigration(r.Context(), nexusURL, req)
	if err != nil {
		slog.Error("failed to create tenant via nexus", "error", err)
		if statusCode == http.StatusConflict {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "tenant provisioning failed")
		return
	}

	// Step 2: Get credentials for the new tenant via service account rotation.
	serviceAccount, err := rotateServiceAccountForMigration(r.Context(), nexusURL, adminKey, tenantID)
	if err != nil {
		slog.Error("failed to rotate service account for new tenant", "tenant_id", tenantID, "error", err)
		writeError(w, http.StatusInternalServerError, "tenant created but database setup failed")
		return
	}

	// Step 3: Connect to the tenant's database.
	nexusHost := os.Getenv("NEXUS_HOST")
	if nexusHost == "" {
		nexusHost = "nexus-gateway"
	}
	nexusPort := os.Getenv("NEXUS_PORT")
	if nexusPort == "" {
		nexusPort = "5433"
	}

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		nexusHost, nexusPort, serviceAccount.Username, serviceAccount.Password, serviceAccount.Database)

	sqlDB, err := sql.Open("postgres", connStr)
	if err != nil {
		slog.Error("failed to open database connection for new tenant", "tenant_id", tenantID, "error", err)
		writeError(w, http.StatusInternalServerError, "tenant created but database setup failed")
		return
	}
	defer sqlDB.Close()

	tenantDB := db.WrapDB(sqlDB)

	// Step 4: Run migrations and generate occurrences for this tenant.
	if err := db.MigrateAndGenerateTenant(tenantDB, tenantID); err != nil {
		// The tenant was created successfully; log the error but do
		// not roll back the registration — migrations can be retried later.
		slog.Error("migration and occurrence generation failed after tenant creation", "tenant_id", tenantID, "error", err)
		writeError(w, http.StatusInternalServerError, "tenant created but database initialisation failed")
		return
	}

	slog.Info("tenant registered and portal schema initialised", "tenant_id", tenantID)
	writeJSON(w, http.StatusCreated, registerWithMigrationResponse{TenantID: tenantID})
}

// createTenantViaNexusForMigration calls the nexus-control registration endpoint and returns
// the new tenant ID. It also returns the HTTP status code from nexus so that
// callers can map it to the appropriate response code.
func createTenantViaNexusForMigration(ctx context.Context, nexusURL string, req registerWithMigrationRequest) (tenantID string, statusCode int, err error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, nexusURL+"/api/v1/register", bytes.NewReader(payload))
	if err != nil {
		return "", 0, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", 0, fmt.Errorf("nexus register: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		var nexusResp nexusRegisterWithMigrationResponse
		if err := json.Unmarshal(body, &nexusResp); err == nil && nexusResp.Error != "" {
			return "", resp.StatusCode, fmt.Errorf("nexus: %s", nexusResp.Error)
		}
		return "", resp.StatusCode, fmt.Errorf("nexus returned status %d", resp.StatusCode)
	}

	var nexusResp nexusRegisterWithMigrationResponse
	if err := json.Unmarshal(body, &nexusResp); err != nil {
		return "", resp.StatusCode, fmt.Errorf("decode nexus response: %w", err)
	}
	return nexusResp.TenantID, resp.StatusCode, nil
}

// rotateServiceAccountForMigration calls the nexus-control service account rotation endpoint
// and returns the credentials for the tenant's service account.
func rotateServiceAccountForMigration(ctx context.Context, nexusURL, adminKey, tenantID string) (*serviceAccountWithMigrationResponse, error) {
	endpoint := fmt.Sprintf("%s/api/v1/admin/tenants/%s/service-account/rotate", nexusURL, tenantID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Admin-API-Key", adminKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("service account rotation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("service account rotation returned status %d: %s", resp.StatusCode, string(body))
	}

	var serviceAccount serviceAccountWithMigrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&serviceAccount); err != nil {
		return nil, fmt.Errorf("decode service account response: %w", err)
	}

	return &serviceAccount, nil
}

// nexusControlURLForMigration returns the nexus-control base URL from the NEXUS_CONTROL_URL
// environment variable, defaulting to http://nexus-control:8080.
func nexusControlURLForMigration() string {
	if u := os.Getenv("NEXUS_CONTROL_URL"); u != "" {
		return u
	}
	return "http://nexus-control:8080"
}
