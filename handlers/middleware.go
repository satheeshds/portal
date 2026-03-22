package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
)

// Response is the standard JSON envelope for all API responses.
type Response struct {
	Data  any    `json:"data"`
	Error string `json:"error,omitempty"`
}

// DB is the shared database connection used by all handlers.
var DB *sql.DB

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

// BasicAuth is middleware that enforces HTTP Basic Authentication.
func BasicAuth(next http.Handler) http.Handler {
	user := os.Getenv("AUTH_USER")
	pass := os.Getenv("AUTH_PASS")

	// If no credentials are configured, skip auth
	if user == "" && pass == "" {
		slog.Warn("AUTH_USER and AUTH_PASS not set, API is unauthenticated")
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="accounting"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// responseRecorder wraps http.ResponseWriter to capture the status code and body for logging.
type responseRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (rr *responseRecorder) WriteHeader(status int) {
	rr.status = status
	rr.ResponseWriter.WriteHeader(status)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	rr.body.Write(b)
	return rr.ResponseWriter.Write(b)
}

// DebugLogger is middleware that logs the full request and response bodies at debug level.
// It is a no-op when debug logging is not enabled.
// WARNING: enabling debug logging will expose full request and response bodies in logs,
// including potentially sensitive data such as passwords and tokens.
func DebugLogger(next http.Handler) http.Handler {
	const maxBodyLog = 64 * 1024 // 64 KB – avoid buffering huge uploads

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slog.Default().Enabled(r.Context(), slog.LevelDebug) {
			next.ServeHTTP(w, r)
			return
		}

		// Capture request body up to the size limit.
		var reqBody []byte
		if r.Body != nil {
			limited := io.LimitReader(r.Body, maxBodyLog)
			var err error
			reqBody, err = io.ReadAll(limited)
			if err != nil {
				slog.DebugContext(r.Context(), "failed to read request body for logging", "error", err)
			}
			// Restore the body so downstream handlers can read it.
			r.Body = io.NopCloser(bytes.NewBuffer(reqBody))
		}

		slog.DebugContext(r.Context(), "incoming request",
			"method", r.Method,
			"url", r.URL.String(),
			"headers", r.Header,
			"body", string(reqBody),
		)

		rr := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rr, r)

		slog.DebugContext(r.Context(), "outgoing response",
			"method", r.Method,
			"url", r.URL.String(),
			"status", rr.status,
			"body", rr.body.String(),
		)
	})
}
