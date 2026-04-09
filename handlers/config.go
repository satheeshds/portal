package handlers

import (
	"os"
	"strings"
)

// Config holds all runtime configuration for the portal handlers.
// It is populated once at startup by calling Configure(ConfigFromEnv()).
type Config struct {
	// NexusControlURL is the base URL of the nexus-control service (no trailing slash).
	NexusControlURL string
	// AdminAPIKey is the admin API key used for nexus-control operations.
	AdminAPIKey string
	// NexusHost is the nexus gateway host used for per-request DB connections.
	// When empty, per-request DB connections are not opened.
	NexusHost string
	// AuthUser and AuthPass are HTTP Basic Auth credentials used as a fallback
	// when NEXUS_CONTROL_URL is not configured.
	AuthUser string
	AuthPass string
}

// cfg is the package-level portal configuration. It is set once at startup
// via Configure and then read by all handlers without further env-var lookups.
var cfg Config

// Configure sets the portal handler configuration. It must be called once at
// startup before any requests are served. It is not safe to call concurrently
// with request handling.
func Configure(c Config) {
	cfg = c
}

// ConfigFromEnv reads the portal configuration from environment variables.
// It is intended to be called once from main during startup.
func ConfigFromEnv() Config {
	return Config{
		NexusControlURL: strings.TrimRight(os.Getenv("NEXUS_CONTROL_URL"), "/"),
		AdminAPIKey:     os.Getenv("ADMIN_API_KEY"),
		NexusHost:       os.Getenv("NEXUS_HOST"),
		AuthUser:        os.Getenv("AUTH_USER"),
		AuthPass:        os.Getenv("AUTH_PASS"),
	}
}
