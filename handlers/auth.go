package handlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/satheeshds/portal/db"
)

// nexusClient is a shared HTTP client. Timeouts are managed at the request level via context
// to allow different limits for registration (5 min) and login (30 sec).
var nexusClient = &http.Client{}

// nexusRegisterClient is a dedicated HTTP client for the long-running Nexus registration
// request. Its transport disables keep-alives (preventing connection-pool reuse for HTTP/1.x)
// and HTTP/2 (via a non-nil empty TLSNextProto map), so EOF errors from intermediate proxies
// dropping idle connections are avoided consistently across both protocols.
var nexusRegisterClient = &http.Client{
	Transport: &http.Transport{
		DisableKeepAlives: true,
		// Setting TLSNextProto to a non-nil empty map is the documented Go mechanism
		// for disabling HTTP/2 (see net/http Transport.TLSNextProto docs: "if not nil,
		// HTTP/2 support is not enabled automatically"). ForceAttemptHTTP2=false is the
		// default and only affects transports that use custom dial functions; it does not
		// disable HTTP/2 on a standard transport.
		TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
	},
}

// Register provisions a new tenant via nexus-control and then initialises the
// portal schema and occurrence generation for that tenant.
//
// @Summary      Register a new tenant
// @Description  Provisions a tenant via the Nexus gateway and initialises the portal schema. Requires NEXUS_CONTROL_URL and ADMIN_API_KEY to be configured.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      registerRequest  true  "Registration data"
// @Success      201   {object}  map[string]string
// @Failure      400   {object}  Response
// @Failure      409   {object}  Response
// @Failure      500   {object}  Response
// @Failure      502   {object}  Response
// @Router       /api/auth/register [post]
func Register(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	// Validate required fields before making any remote calls.
	var req registerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OrgName == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "org_name, email, and password are required")
		return
	}

	base := cfg.NexusControlURL
	if base == "" {
		writeError(w, http.StatusServiceUnavailable, "NEXUS_CONTROL_URL is not configured")
		return
	}

	adminKey := cfg.AdminAPIKey
	if adminKey == "" {
		slog.Error("ADMIN_API_KEY is not set")
		writeError(w, http.StatusInternalServerError, "server configuration error")
		return
	}

	// Step 1: Register the tenant via nexus-control.
	target := fmt.Sprintf("%s/api/v1/register", base)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create register request")
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := nexusRegisterClient.Do(httpReq)
	if err != nil {
		slog.Error("nexus register request failed", "error", err)
		writeError(w, http.StatusBadGateway, "nexus gateway unavailable")
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// Proxy non-201 responses from nexus (e.g. 400, 409) directly to the client.
	if resp.StatusCode != http.StatusCreated {
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(respBody)
		return
	}

	// Parse the tenant ID returned by nexus.
	var nexusResp struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.Unmarshal(respBody, &nexusResp); err != nil || nexusResp.TenantID == "" {
		slog.Error("failed to parse nexus registration response", "error", err)
		writeError(w, http.StatusBadGateway, "invalid response from nexus gateway")
		return
	}

	tenantID := nexusResp.TenantID

	// Step 2: Rotate service account to obtain tenant-specific DB credentials.
	creds, err := db.RotateTenantServiceAccount(base, adminKey, tenantID)
	if err != nil {
		slog.Error("failed to rotate service account for new tenant", "tenant_id", tenantID, "error", err)
		// Tenant was created; still return 201. The platform service will retry schema init.
		writeJSON(w, http.StatusCreated, map[string]string{"tenant_id": tenantID})
		return
	}

	// Step 3: Connect to the tenant's database.
	tenantDB, err := db.OpenWithCredentials(creds.Username, creds.Password)
	if err != nil {
		slog.Error("failed to open database connection for new tenant", "tenant_id", tenantID, "error", err)
		// Tenant was created; still return 201. The platform service will retry.
		writeJSON(w, http.StatusCreated, map[string]string{"tenant_id": tenantID})
		return
	}
	defer tenantDB.Close()

	// Step 4: Run schema migrations for this tenant.
	// Occurrence generation is handled by the platform service on its next cycle.
	if err := db.MigrateTenant(tenantDB, tenantID); err != nil {
		slog.Error("schema migration failed after tenant creation", "tenant_id", tenantID, "error", err)
		// Tenant was created; still return 201. The platform service will retry.
	}

	slog.Info("tenant registered and portal schema initialised", "tenant_id", tenantID)
	writeJSON(w, http.StatusCreated, map[string]string{"tenant_id": tenantID})
}

// Login proxies a login request to the Nexus gateway and returns the JWT token.
//
// @Summary      Login
// @Description  Authenticates with the Nexus gateway using email and password. Returns a JWT token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      loginRequest  true  "Login credentials"
// @Success      200   {object}  map[string]string
// @Failure      400   {object}  Response
// @Failure      401   {object}  Response
// @Failure      502   {object}  Response
// @Router       /api/auth/login [post]
func Login(w http.ResponseWriter, r *http.Request) {
	base := cfg.NexusControlURL
	if base == "" {
		writeError(w, http.StatusServiceUnavailable, "NEXUS_CONTROL_URL is not configured")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	target := fmt.Sprintf("%s/api/v1/login", base)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create login request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := nexusClient.Do(req)
	if err != nil {
		slog.Error("nexus login request failed", "error", err)
		writeError(w, http.StatusBadGateway, "nexus gateway unavailable")
		return
	}
	defer resp.Body.Close()

	// Nexus returns {"token": "..."} directly (not wrapped); re-wrap in our standard envelope.
	if resp.StatusCode == http.StatusOK {
		var nexusResp map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&nexusResp); err != nil {
			writeError(w, http.StatusBadGateway, "invalid response from nexus gateway")
			return
		}
		writeJSON(w, http.StatusOK, nexusResp)
		return
	}

	proxyResponse(w, resp)
}

// proxyResponse copies the Nexus response status and body to the client,
// preserving the content-type header.
func proxyResponse(w http.ResponseWriter, resp *http.Response) {
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// registerRequest documents the fields accepted by the register endpoint.
type registerRequest struct {
	OrgName  string `json:"org_name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginRequest documents the fields accepted by the login endpoint.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
