package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"

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
// connection has been configured. A per-request connection will be injected here
// once JWT-based authentication is implemented.
func DBRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if DB == nil {
			writeError(w, http.StatusServiceUnavailable, "database connection not available")
			return
		}
		next.ServeHTTP(w, r)
	})
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
			w.Header().Set("WWW-Authenticate", `Basic realm="portal"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
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
