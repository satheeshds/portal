package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// makeJWT builds a minimal JWT with the given tenant_id claim and expiry
// (seconds from now). The signature is intentionally empty – validateNexusToken
// only checks structure and the exp claim.
func makeJWT(tenantID string, expiresInSecs int64) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]interface{}{
		"tenant_id": tenantID,
		"exp":       time.Now().Unix() + expiresInSecs,
	})
	claims := base64.RawURLEncoding.EncodeToString(payload)
	return fmt.Sprintf("%s.%s.fakesig", header, claims)
}

// okHandler is a trivial http.Handler that always returns 200 OK.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// bearerRequest builds a test request with a Bearer token.
func bearerRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// basicAuthRequest builds a test request with HTTP Basic Auth credentials.
func basicAuthRequest(user, pass string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.SetBasicAuth(user, pass)
	return req
}

// ── No-auth (unconfigured) ────────────────────────────────────────────────────

func TestBearerAuth_NoConfig_AllowsUnauthenticatedAccess(t *testing.T) {
	withTestConfig(t, Config{})
	h := BearerAuth(okHandler)

	// No auth credentials required when nothing is configured; the middleware
	// passes the request through to the next handler unconditionally.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// ── Static Basic Auth fallback (no NEXUS_CONTROL_URL) ────────────────────────

func TestBearerAuth_StaticBasicAuth_Valid(t *testing.T) {
	withTestConfig(t, Config{AuthUser: "admin", AuthPass: "secret"})
	h := BearerAuth(okHandler)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, basicAuthRequest("admin", "secret"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestBearerAuth_StaticBasicAuth_InvalidCredentials(t *testing.T) {
	withTestConfig(t, Config{AuthUser: "admin", AuthPass: "secret"})
	h := BearerAuth(okHandler)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, basicAuthRequest("admin", "wrong"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestBearerAuth_StaticBasicAuth_MissingCredentials(t *testing.T) {
	withTestConfig(t, Config{AuthUser: "admin", AuthPass: "secret"})
	h := BearerAuth(okHandler)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ── JWT Bearer token (with NEXUS_CONTROL_URL, no NEXUS_HOST) ─────────────────

func TestBearerAuth_JWT_ValidToken_NoNexusHost(t *testing.T) {
	withTestConfig(t, Config{NexusControlURL: "http://nexus.example.com"})
	h := BearerAuth(okHandler)

	token := makeJWT("tenant_abc", 3600)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, bearerRequest(token))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// TestBearerAuth_JWT_RawToken verifies that a raw JWT (without "Bearer " prefix)
// is accepted — this is what Swagger UI sends when using an apiKey security scheme.
func TestBearerAuth_JWT_RawToken(t *testing.T) {
	withTestConfig(t, Config{NexusControlURL: "http://nexus.example.com"})
	h := BearerAuth(okHandler)

	token := makeJWT("tenant_abc", 3600)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", token) // raw token, no "Bearer " prefix
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for raw JWT token, got %d", rec.Code)
	}
}

func TestBearerAuth_JWT_ExpiredToken(t *testing.T) {
	withTestConfig(t, Config{NexusControlURL: "http://nexus.example.com"})
	h := BearerAuth(okHandler)

	// Token expired 1 second ago.
	token := makeJWT("tenant_abc", -1)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, bearerRequest(token))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", rec.Code)
	}
}

func TestBearerAuth_JWT_MalformedToken(t *testing.T) {
	withTestConfig(t, Config{NexusControlURL: "http://nexus.example.com"})
	h := BearerAuth(okHandler)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, bearerRequest("not-a-jwt"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for malformed token, got %d", rec.Code)
	}
}

func TestBearerAuth_MissingAuth_WithNexusControl(t *testing.T) {
	withTestConfig(t, Config{NexusControlURL: "http://nexus.example.com"})
	h := BearerAuth(okHandler)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when no auth provided, got %d", rec.Code)
	}
}

// ── Service account via Basic Auth (with NEXUS_CONTROL_URL + NEXUS_HOST) ─────

func TestBearerAuth_ServiceAccount_WithoutNexusHost_Returns503(t *testing.T) {
	// NEXUS_CONTROL_URL is set but NEXUS_HOST is not – service account auth
	// requires NEXUS_HOST to open a per-request DB connection.
	withTestConfig(t, Config{NexusControlURL: "http://nexus.example.com"})
	h := BearerAuth(okHandler)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, basicAuthRequest("svc_id_123", "some-api-key"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when NEXUS_HOST is not set, got %d", rec.Code)
	}
}

func TestBearerAuth_ServiceAccount_LazyPoolCreation(t *testing.T) {
	// When NEXUS_HOST is configured, the middleware creates a connection pool
	// lazily (no immediate TCP connection is made by OpenWithCredentials).
	// Credential validation happens inside the Nexus DB gateway on the first
	// query, which is outside the middleware's scope. The middleware's job is
	// only to route the request to the correct DB connection pool; it returns
	// 200 here because the pool creation itself succeeds regardless of whether
	// the credentials are valid.
	t.Setenv("NEXUS_HOST", "127.0.0.1")
	t.Setenv("NEXUS_PORT", "15433") // non-listening port
	withTestConfig(t, Config{
		NexusControlURL: "http://nexus.example.com",
		NexusHost:       "127.0.0.1",
	})
	h := BearerAuth(okHandler)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, basicAuthRequest("svc_id_123", "bad-api-key"))
	// Pool creation is lazy – no auth error is surfaced here.
	// The Nexus gateway validates credentials when the first query runs.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (lazy pool creation), got %d", rec.Code)
	}
}

func TestBearerAuth_ServiceAccount_EmptyCredentials_Returns401(t *testing.T) {
	// Empty username or password should be rejected.
	withTestConfig(t, Config{
		NexusControlURL: "http://nexus.example.com",
		NexusHost:       "127.0.0.1",
	})
	h := BearerAuth(okHandler)

	// Basic Auth with empty password.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.SetBasicAuth("svc_id", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	// Empty apiKey means the Basic Auth condition (serviceID != "" && apiKey != "") fails.
	// Falls through to the final "missing or invalid authentication" check.
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for empty api key, got %d", rec.Code)
	}
}
