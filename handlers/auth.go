package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// nexusURL returns the configured Nexus gateway base URL, trimming any trailing slash.
func nexusURL() string {
	return strings.TrimRight(os.Getenv("NEXUS_URL"), "/")
}

// Register proxies a tenant registration request to the Nexus gateway.
//
// @Summary      Register a new tenant
// @Description  Proxies registration to the Nexus gateway. Requires NEXUS_URL to be configured.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      registerRequest  true  "Registration data"
// @Success      201   {object}  map[string]string
// @Failure      400   {object}  Response
// @Failure      409   {object}  Response
// @Failure      502   {object}  Response
// @Router       /api/auth/register [post]
func Register(w http.ResponseWriter, r *http.Request) {
	base := nexusURL()
	if base == "" {
		writeError(w, http.StatusServiceUnavailable, "NEXUS_URL is not configured")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	target := fmt.Sprintf("%s/api/v1/register", base)
	resp, err := http.Post(target, "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		slog.Error("nexus register request failed", "error", err)
		writeError(w, http.StatusBadGateway, "nexus gateway unavailable")
		return
	}
	defer resp.Body.Close()

	proxyResponse(w, resp)
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
// @Router       /auth/login [post]
func Login(w http.ResponseWriter, r *http.Request) {
	base := nexusURL()
	if base == "" {
		writeError(w, http.StatusServiceUnavailable, "NEXUS_URL is not configured")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	target := fmt.Sprintf("%s/api/v1/login", base)
	resp, err := http.Post(target, "application/json", bytes.NewReader(body)) //nolint:noctx
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
