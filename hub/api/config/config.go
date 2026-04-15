package config

import (
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	APIKey          string
	ClickHouseDSN   string
	KafkaBrokers    []string
	KafkaTopic      string
	Port            string
}

// Load reads configuration from environment variables, optionally loading a
// .env file first if one is present in the working directory.
func Load() *Config {
	// Best-effort .env load; not an error if the file is absent.
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		slog.Warn("could not load .env file", "err", err)
	}

	cfg := &Config{
		APIKey:        getEnv("API_KEY", ""),
		ClickHouseDSN: getEnv("CLICKHOUSE_DSN", "clickhouse://netscope:netscope_pass@clickhouse:9000/netscope"),
		KafkaTopic:    getEnv("KAFKA_TOPIC", "netscope.flows"),
		Port:          getEnv("PORT", "8080"),
	}

	brokerStr := getEnv("KAFKA_BROKERS", "redpanda:9092")
	cfg.KafkaBrokers = splitAndTrim(brokerStr, ",")

	if cfg.APIKey == "" {
		slog.Warn("API_KEY env var is not set — all requests will be rejected")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
