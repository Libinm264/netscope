package config

import (
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	APIKey         string
	ClickHouseDSN  string
	KafkaBrokers   []string
	KafkaTopic     string
	Port           string
	// AllowedOrigins is the CORS allowed-origins list (comma-separated).
	// Defaults to "*" for local dev; set to your domain(s) in production.
	AllowedOrigins string

	// Geo-IP database paths (MaxMind GeoLite2 .mmdb files).
	// Leave empty to disable geo enrichment.
	GeoIPCityDB string
	GeoIPAsnDB  string

	// AbuseIPDB API key for real-time threat lookups (optional).
	AbuseIPDBKey string

	// Path to a CIDR blocklist file for the threat scorer (optional).
	ThreatBlocklist string
}

// Load reads configuration from environment variables, optionally loading a
// .env file first if one is present in the working directory.
func Load() *Config {
	// Best-effort .env load; not an error if the file is absent.
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		slog.Warn("could not load .env file", "err", err)
	}

	cfg := &Config{
		APIKey:          getEnv("API_KEY", ""),
		ClickHouseDSN:   getEnv("CLICKHOUSE_DSN", "clickhouse://netscope:netscope_pass@clickhouse:9000/netscope"),
		KafkaTopic:      getEnv("KAFKA_TOPIC", "netscope.flows"),
		Port:            getEnv("PORT", "8080"),
		AllowedOrigins:  getEnv("ALLOWED_ORIGINS", "*"),
		GeoIPCityDB:     getEnv("GEOIP_CITY_DB", ""),
		GeoIPAsnDB:      getEnv("GEOIP_ASN_DB", ""),
		AbuseIPDBKey:    getEnv("ABUSEIPDB_KEY", ""),
		ThreatBlocklist: getEnv("THREAT_BLOCKLIST", ""),
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
