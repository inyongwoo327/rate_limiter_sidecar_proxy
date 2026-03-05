package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	// ProxyPort is the port the rate-limiting proxy listens on.
	// Clients send their requests here.
	ProxyPort string

	// MetricsPort is the port for /metrics and /healthz endpoints.
	// Prometheus scrapes this port to collect statistics.
	MetricsPort string

	// DownstreamURL is the address of the real service we forward requests to.
	// e.g. "http://localhost:80" — the nginx container in the same pod.
	DownstreamURL string

	// Capacity is the maximum number of tokens a client bucket can hold.
	// Think of it as the "burst limit" — how many requests a client can
	// fire in a single instant before being throttled.
	Capacity int

	// RefillRate is how many tokens are added back per second.
	// e.g. RefillRate=2 means a client recovers 2 "request slots" every second.
	RefillRate float64

	// ClientIDHeader is the HTTP header we look at to identify a client.
	// If this header is missing, we fall back to the client's IP address.
	ClientIDHeader string
}

func Load() *Config {
	return &Config{
		ProxyPort:      getEnv("PROXY_PORT", "8080"),
		MetricsPort:    getEnv("METRICS_PORT", "9090"),
		DownstreamURL:  getEnv("DOWNSTREAM_URL", "http://localhost:8081"),
		Capacity:       getEnvInt("RATE_LIMIT_CAPACITY", 10),
		RefillRate:     getEnvFloat("RATE_LIMIT_REFILL_RATE", 2.0),
		ClientIDHeader: getEnv("CLIENT_ID_HEADER", "X-Client-ID"),
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// getEnv reads a string environment variable.
// If the variable is not set, it returns the provided fallback value.
func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// getEnvInt reads an integer environment variable.
// If the variable is missing or cannot be parsed as a number, the fallback is used.
func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.Atoi(val)
		if err != nil {
			// The value was set but is not a valid integer — warn and use default.
			log.Printf("WARN: %s=%q is not a valid integer, using default %d", key, val, fallback)
			return fallback
		}
		return parsed
	}
	return fallback
}

// getEnvFloat reads a float64 environment variable.
// Used for RefillRate which can be a decimal like 0.5 (1 token every 2 seconds).
func getEnvFloat(key string, fallback float64) float64 {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.ParseFloat(val, 64)
		if err != nil {
			log.Printf("WARN: %s=%q is not a valid float, using default %f", key, val, fallback)
			return fallback
		}
		return parsed
	}
	return fallback
}
