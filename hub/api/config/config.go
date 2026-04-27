package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// knownDefaultDSN is the well-known insecure ClickHouse DSN shipped in the
// repository.  We detect it at startup to warn operators (and refuse to start
// in PRODUCTION mode).
const knownDefaultDSN = "clickhouse://netscope:netscope_pass@clickhouse:9000/netscope"

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	APIKey         string
	ClickHouseDSN  string
	KafkaBrokers   []string
	KafkaTopic     string
	// KafkaGroupID is the Kafka consumer group name.  Defaults to "netscope-ch-writer".
	// Set KAFKA_GROUP_ID to run multiple hub instances without duplicate processing
	// (Enterprise horizontal scaling).
	KafkaGroupID   string
	Port           string
	// AllowedOrigins is the CORS allowed-origins list (comma-separated).
	// Defaults to "*" for local dev; MUST be set to your domain(s) in production.
	AllowedOrigins string

	// GeoIP database paths (MaxMind GeoLite2 .mmdb files).
	GeoIPCityDB string
	GeoIPAsnDB  string

	// AbuseIPDB API key for real-time threat lookups (optional).
	AbuseIPDBKey string

	// Path to a CIDR blocklist file for the threat scorer (optional).
	ThreatBlocklist string

	// MetricsToken, when non-empty, requires "Authorization: Bearer <token>"
	// on requests to the /metrics endpoint.  Leave empty to allow unauthenticated
	// scraping (suitable only when Prometheus is on a private network).
	MetricsToken string

	// SMTP email delivery for alert notifications.
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	// AppURL is the public base URL of the hub (used in email alert links).
	AppURL string
	// OrgName is a display name for the organisation (used in alert emails/UI).
	OrgName string

	// Scheduled report settings.
	// ReportEmail is the recipient of daily/weekly summaries; empty disables reports.
	ReportEmail    string
	// ReportSchedule is "daily" (default) or "weekly".
	ReportSchedule string

	// ── Enterprise Edition ────────────────────────────────────────────────────

	// EnterpriseLicenseKey is the JWT license key (ENTERPRISE_LICENSE_KEY env var).
	// If empty, the hub runs in community mode (10-agent soft cap, no SSO).
	EnterpriseLicenseKey string

	// EnterpriseLicenseSigningKey is the HMAC-SHA256 secret used to verify
	// license JWTs (ENTERPRISE_LICENSE_SIGNING_KEY env var).
	// Defaults to the embedded dev key when unset.
	EnterpriseLicenseSigningKey string

	// FrontendURL is the base URL of the Next.js UI, used for SSO redirects
	// after SAML/OIDC callbacks complete (FRONTEND_URL env var).
	FrontendURL string

	// SCIMBearerToken is the long-lived token that Okta / Azure AD present in
	// the Authorization: Bearer header when calling SCIM 2.0 endpoints.
	// Generate a random string (e.g. openssl rand -hex 32) and set both here
	// and in your IdP's SCIM provisioning configuration.
	SCIMBearerToken string

	// SSOClientSecret is the OAuth2 / OIDC client secret for the SSO application
	// registered with the IdP (Okta, Azure AD, Dex, Google, etc.).
	// Never stored in ClickHouse — only kept in memory at runtime.
	SSOClientSecret string

	// AdminEmail / AdminPassword seed the initial local admin account on first
	// startup.  Plaintext password is hashed and stored; these env vars can be
	// removed afterwards.  Ignored when the account already exists.
	AdminEmail    string
	AdminPassword string

	// DemoEnabled, when true, exposes POST /api/v1/auth/demo which creates a
	// short-lived read-only session so prospects can evaluate the UI without
	// creating an account.  Set DEMO_ENABLED=true in docker-compose for try-it
	// deployments; leave unset (default false) for production installs.
	DemoEnabled bool

	// ── Social login (OAuth2) ─────────────────────────────────────────────────

	// GoogleClientID / GoogleClientSecret enable "Sign in with Google".
	// Register a Web application at https://console.cloud.google.com/apis/credentials
	// and add {APP_URL}/api/v1/auth/google/callback as an authorised redirect URI.
	// Leave both empty to hide the Google button on the login page.
	GoogleClientID     string
	GoogleClientSecret string
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
		ClickHouseDSN:   getEnv("CLICKHOUSE_DSN", knownDefaultDSN),
		KafkaTopic:      getEnv("KAFKA_TOPIC", "netscope.flows"),
		KafkaGroupID:    getEnv("KAFKA_GROUP_ID", "netscope-ch-writer"),
		Port:            getEnv("PORT", "8080"),
		AllowedOrigins:  getEnv("ALLOWED_ORIGINS", "*"),
		GeoIPCityDB:     getEnv("GEOIP_CITY_DB", ""),
		GeoIPAsnDB:      getEnv("GEOIP_ASN_DB", ""),
		AbuseIPDBKey:    getEnv("ABUSEIPDB_KEY", ""),
		ThreatBlocklist: getEnv("THREAT_BLOCKLIST", ""),
		MetricsToken:    getEnv("METRICS_TOKEN", ""),
		SMTPHost:        getEnv("SMTP_HOST", ""),
		SMTPPort:        getEnvInt("SMTP_PORT", 587),
		SMTPUser:        getEnv("SMTP_USER", ""),
		SMTPPassword:    getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:        getEnv("SMTP_FROM", "noreply@netscope.local"),
		AppURL:          getEnv("APP_URL", "http://localhost:8080"),
		OrgName:         getEnv("ORG_NAME", "NetScope"),
		ReportEmail:     getEnv("REPORT_EMAIL", ""),
		ReportSchedule:  getEnv("REPORT_SCHEDULE", "daily"),

		EnterpriseLicenseKey:        getEnv("ENTERPRISE_LICENSE_KEY", ""),
		EnterpriseLicenseSigningKey: getEnv("ENTERPRISE_LICENSE_SIGNING_KEY", ""),
		FrontendURL:                 getEnv("FRONTEND_URL", "http://localhost:3000"),
		SCIMBearerToken:             getEnv("SCIM_BEARER_TOKEN", ""),
		SSOClientSecret:             getEnv("SSO_CLIENT_SECRET", ""),
		AdminEmail:                  getEnv("ADMIN_EMAIL", ""),
		AdminPassword:               getEnv("ADMIN_PASSWORD", ""),
		DemoEnabled:                 os.Getenv("DEMO_ENABLED") == "true",

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
	}

	brokerStr := getEnv("KAFKA_BROKERS", "redpanda:9092")
	cfg.KafkaBrokers = splitAndTrim(brokerStr, ",")

	// ── Security validation ───────────────────────────────────────────────────

	production := os.Getenv("PRODUCTION") == "true"

	if cfg.APIKey == "" {
		if production {
			slog.Error("PRODUCTION: API_KEY is required — refusing to start")
			os.Exit(1)
		}
		slog.Warn("⚠  SECURITY: API_KEY is not set — all API requests will be rejected")
	}

	if cfg.ClickHouseDSN == knownDefaultDSN {
		if production {
			slog.Error("PRODUCTION: CLICKHOUSE_DSN is using the public default credentials — refusing to start. Set CLICKHOUSE_DSN.")
			os.Exit(1)
		}
		slog.Warn("⚠  SECURITY: Using default ClickHouse credentials (publicly known). Set CLICKHOUSE_DSN before deploying.")
	}

	if cfg.AllowedOrigins == "*" {
		if production {
			slog.Error("PRODUCTION: ALLOWED_ORIGINS is set to wildcard (*) — refusing to start. Set ALLOWED_ORIGINS to your domain.")
			os.Exit(1)
		}
		slog.Warn("⚠  SECURITY: CORS is open to all origins (*). Set ALLOWED_ORIGINS=https://your-domain.com for production.")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
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
