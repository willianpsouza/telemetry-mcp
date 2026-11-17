package config

import (
	"os"
	"time"
)

// Config holds runtime settings resolved from environment variables.
type Config struct {
	Transport       string // "stdio" (default) or "http"
	HTTPAddr        string // listen address when Transport == "http"
	PrometheusURL   string
	JaegerURL       string
	LokiURL         string
	DefaultLookback time.Duration
	HTTPTimeout     time.Duration
	AuthToken       string // when set (http transport), require Authorization: Bearer <token>
}

// Load reads configuration from the environment, applying sensible defaults.
func Load() Config {
	return Config{
		Transport:       env("MCP_TRANSPORT", "stdio"),
		HTTPAddr:        env("MCP_HTTP_ADDR", ":8080"),
		PrometheusURL:   env("MCP_PROMETHEUS_URL", "http://localhost:9090"),
		JaegerURL:       env("MCP_JAEGER_URL", "http://localhost:16686"),
		LokiURL:         env("MCP_LOKI_URL", "http://localhost:3100"),
		DefaultLookback: envDuration("MCP_DEFAULT_LOOKBACK", time.Hour),
		HTTPTimeout:     envDuration("MCP_HTTP_TIMEOUT", 30*time.Second),
		AuthToken:       os.Getenv("MCP_AUTH_TOKEN"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
