package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/satheeshds/portal/db"
)

// maxBodyLog is the maximum number of bytes captured from request/response bodies for debug logging.
const maxBodyLog = 64 * 1024 // 64 KB

// Response is the standard JSON envelope for all API responses.
type Response struct {
	Data  any    `json:"data"`
	Error string `json:"error,omitempty"`
}

// DB is the shared database connection used by all handlers.
var DB *db.PortalDB

type contextKey int

const dbKey contextKey = 0

// withDB stores a per-request PortalDB in the context.
func withDB(ctx context.Context, d *db.PortalDB) context.Context {
	return context.WithValue(ctx, dbKey, d)
}

// getDB returns the per-request PortalDB from the context, falling back to the
// global DB so that tests that set the global variable continue to work.
func getDB(r *http.Request) *db.PortalDB {
	if d, ok := r.Context().Value(dbKey).(*db.PortalDB); ok && d != nil {
		return d
	}
	return DB
}

// extractTenantID parses the JWT payload (second dot-separated part) and returns
// the tenant_id claim value, or ("", false) if it cannot be extracted.
func extractTenantID(token string) (string, bool) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var claims struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.TenantID == "" {
		return "", false
	}
	return claims.TenantID, true
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{Data: data})
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{Error: msg})
}

// DBRequired is middleware that returns 503 Service Unavailable when no database
// connection has been configured.
func DBRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if getDB(r) == nil {
			writeError(w, http.StatusServiceUnavailable, "database connection not available")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// BasicAuth is middleware that enforces HTTP Basic Authentication.
func BasicAuth(next http.Handler) http.Handler {
	user := cfg.AuthUser
	pass := cfg.AuthPass

	// If no credentials are configured, skip auth
	if user == "" && pass == "" {
		slog.Warn("AUTH_USER and AUTH_PASS not set, API is unauthenticated")
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="portal"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// BearerAuth is middleware that enforces Bearer token authentication.
// When NEXUS_CONTROL_URL is set it supports two authentication methods:
//  1. JWT Bearer token – the existing user login flow.
//  2. HTTP Basic Auth – service account authentication where the username is
//     the service_id and the password is the service_api_key. The Nexus
//     gateway validates the credentials via bcrypt when the per-request DB
//     connection is opened (requires NEXUS_HOST to be configured).
//
// If neither NEXUS_CONTROL_URL nor AUTH_USER/AUTH_PASS are configured the
// middleware falls back to the unauthenticated (open) behaviour and logs a
// warning.
func BearerAuth(next http.Handler) http.Handler {
	nexus := cfg.NexusControlURL
	authUser := cfg.AuthUser
	authPass := cfg.AuthPass
	nexusHost := cfg.NexusHost

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If nothing is configured, warn and pass through (same as original BasicAuth).
		if nexus == "" && authUser == "" && authPass == "" {
			slog.Warn("no auth configured (NEXUS_CONTROL_URL, AUTH_USER, AUTH_PASS), API is unauthenticated")
			next.ServeHTTP(w, r)
			return
		}

		// Prefer Bearer token when NEXUS_CONTROL_URL is configured.
		if nexus != "" {
			authHeader := r.Header.Get("Authorization")

			// ── JWT Bearer token ──────────────────────────────────────────────
			// Accept both "Bearer <token>" and a raw token (e.g. from Swagger UI
			// apiKey auth which sends the header value as-is without the prefix).
			// Basic auth headers are left to the service-account path below.
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != "" && !strings.HasPrefix(authHeader, "Basic ") {
				if !validateNexusToken(token) {
					writeError(w, http.StatusUnauthorized, "unauthorized")
					return
				}

				// Open a per-request DB connection to the Nexus gateway when
				// NEXUS_HOST is configured, using tenant_id as the PostgreSQL
				// username and the JWT token as the password. The connection is
				// closed explicitly after the handler returns.
				// Note: the DSN embeds the JWT token; avoid logging it in error paths.
				var reqDB *db.PortalDB
				if nexusHost != "" {
					tenantID, ok := extractTenantID(token)
					if !ok {
						writeError(w, http.StatusUnauthorized, "unauthorized")
						return
					}

					opened, err := db.OpenWithCredentials(tenantID, token)
					if err != nil {
						slog.WarnContext(r.Context(), "failed to open per-request DB connection",
							"tenant_id", tenantID, "error", err)
						writeError(w, http.StatusServiceUnavailable, "service unavailable")
						return
					}

					reqDB = opened
					r = r.WithContext(withDB(r.Context(), reqDB))
				}

				next.ServeHTTP(w, r)

				if reqDB != nil {
					reqDB.Close()
				}
				return
			}

			// ── Service account via HTTP Basic Auth ───────────────────────────
			// The service_id is used as the PostgreSQL username and the
			// service_api_key as the password. The Nexus DB gateway validates
			// the credentials using bcrypt when the connection is established.
			if serviceID, apiKey, ok := r.BasicAuth(); ok && serviceID != "" && apiKey != "" {
				if nexusHost == "" {
					slog.WarnContext(r.Context(), "service account authentication requires NEXUS_HOST to be configured")
					writeError(w, http.StatusServiceUnavailable, "service unavailable")
					return
				}

				opened, err := db.OpenWithCredentials(serviceID, apiKey)
				if err != nil {
					slog.WarnContext(r.Context(), "failed to open per-request DB connection for service account",
						"service_id", serviceID, "error", err)
					writeError(w, http.StatusServiceUnavailable, "service unavailable")
					return
				}

				r = r.WithContext(withDB(r.Context(), opened))
				next.ServeHTTP(w, r)
				opened.Close()
				return
			}

			writeError(w, http.StatusUnauthorized, "missing or invalid authentication")
			return
		}

		// Fall back to Basic Auth when AUTH_USER/AUTH_PASS are set.
		u, p, ok := r.BasicAuth()
		if !ok || u != authUser || p != authPass {
			w.Header().Set("WWW-Authenticate", `Basic realm="portal"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// validateNexusToken checks whether a Bearer token has a valid JWT structure and
// has not expired. It does NOT verify the cryptographic signature — full signature
// verification happens at the Nexus layer when the token is forwarded for data
// operations. This guard prevents obviously invalid or expired tokens from passing.
// A missing or zero exp claim is treated as invalid.
func validateNexusToken(token string) bool {
	if token == "" {
		return false
	}
	// A JWT consists of three base64url-encoded parts separated by dots.
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return false
	}
	// Decode the claims (second part). RawURLEncoding handles unpadded base64url.
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return false
	}
	// Require a present (non-zero) exp claim and reject expired tokens.
	if claims.Exp == 0 || time.Now().Unix() >= claims.Exp {
		slog.Debug("rejected bearer token: missing or expired exp claim", "exp", claims.Exp)
		return false
	}
	return true
}

// responseRecorder wraps http.ResponseWriter to capture the status code and body for logging.
// It preserves optional interfaces (http.Flusher, http.Hijacker) of the underlying writer.
type responseRecorder struct {
	http.ResponseWriter
	status    int
	body      bytes.Buffer
	truncated bool
}

func (rr *responseRecorder) WriteHeader(status int) {
	rr.status = status
	rr.ResponseWriter.WriteHeader(status)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	// Write to the underlying writer first; record only the bytes actually sent.
	n, err := rr.ResponseWriter.Write(b)
	if n > 0 && !rr.truncated {
		remaining := maxBodyLog - rr.body.Len()
		if n <= remaining {
			rr.body.Write(b[:n])
		} else {
			rr.body.Write(b[:remaining])
			rr.truncated = true
		}
	}
	return n, err
}

// Flush implements http.Flusher by delegating to the underlying writer if it supports it.
func (rr *responseRecorder) Flush() {
	if f, ok := rr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker by delegating to the underlying writer if it supports it.
func (rr *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rr.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacking not supported by underlying ResponseWriter of type %T", rr.ResponseWriter)
}

// Unwrap returns the underlying http.ResponseWriter, enabling middleware to inspect it.
func (rr *responseRecorder) Unwrap() http.ResponseWriter {
	return rr.ResponseWriter
}

// sensitiveHeaders lists header names whose values are redacted before debug logging.
var sensitiveHeaders = []string{"Authorization", "Cookie", "Set-Cookie", "X-Auth-Token"}

// redactHeaders returns a shallow clone of h with sensitive header values replaced by [REDACTED].
func redactHeaders(h http.Header) http.Header {
	clone := h.Clone()
	for _, key := range sensitiveHeaders {
		if len(clone.Values(key)) > 0 {
			clone.Del(key)
			clone.Set(key, "[REDACTED]")
		}
	}
	return clone
}

// RequestLogger is middleware that logs the full request and response bodies at debug level.
// It is a no-op when debug logging is not enabled.
// WARNING: enabling debug logging will expose full request and response bodies in logs,
// including potentially sensitive data such as passwords and tokens.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slog.Default().Enabled(r.Context(), slog.LevelDebug) {
			next.ServeHTTP(w, r)
			return
		}

		// Capture up to maxBodyLog bytes from the request body for logging, then rebuild
		// r.Body so downstream handlers see the complete original stream (prefix + remainder).
		var reqBody []byte
		if r.Body != nil {
			origBody := r.Body
			limited := io.LimitReader(origBody, maxBodyLog)
			var err error
			reqBody, err = io.ReadAll(limited)
			if err != nil {
				slog.DebugContext(r.Context(), "failed to read request body for logging", "error", err)
			}
			// Replay captured prefix followed by the rest of the original body.
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(reqBody), origBody))
		}

		slog.DebugContext(r.Context(), "incoming request",
			"method", r.Method,
			"url", r.URL.String(),
			"headers", redactHeaders(r.Header),
			"body", string(reqBody),
		)

		rr := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rr, r)

		respBody := rr.body.String()
		if rr.truncated {
			respBody += " [truncated]"
		}
		slog.DebugContext(r.Context(), "outgoing response",
			"method", r.Method,
			"url", r.URL.String(),
			"status", rr.status,
			"body", respBody,
		)
	})
}
