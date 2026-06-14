package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv        string
	HTTPAddr      string
	PublicBaseURL string
	DatabaseURL   string
	EncryptionKey string
	// AllowPrivateProviderInstanceURLs disables SSRF-style host blocking for provider instance URLs (local dev only).
	AllowPrivateProviderInstanceURLs bool
	AllowedOrigins                   []string
	RateLimitPerMinute               int
	// RateLimitAuthenticatedPerMinute caps Bearer-authenticated API traffic (SPA, tokens). 0 = derive on load.
	RateLimitAuthenticatedPerMinute int
	SchedulerPollInterval                time.Duration
	SchedulerMetricsSyncInterval         time.Duration
	SchedulerAccountHealthInterval       time.Duration
	SchedulerExternalPostImportInterval  time.Duration
	SchedulerRSSImportInterval           time.Duration
	SchedulerWorkers                     int

	// LogLevel: debug, info, warn, error — empty means derive from AppEnv (development=debug, production=info).
	LogLevel string
	// LogFormat: json (machine-friendly, default in production) or text (human-friendly, default in development).
	LogFormat string

	OIDCIssuerURL    string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURI  string

	// SessionTTL is the rolling idle lifetime of a web session (cookie + the
	// __web_session token). Each request extends it; an unused session expires.
	SessionTTL time.Duration

	BootstrapAdminEmail string
	BootstrapAdminName  string
	BootstrapAdminToken string
	// BootstrapEnabled allows the recovery bootstrap tab when BootstrapAdminToken is set (after first users exist).
	BootstrapEnabled bool

	MastodonAppName       string
	MastodonRedirectURI   string
	MastodonWebsite       string
	MastodonDefaultScopes []string

	// MCP server configuration
	MCPEnabled            bool
	MCPRateLimitPerMinute int
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:                           strings.TrimSpace(getEnv("APP_ENV", "development")),
		HTTPAddr:                         getEnv("HTTP_ADDR", ":8080"),
		PublicBaseURL:                    getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),
		DatabaseURL:                      getEnv("DATABASE_URL", "file:./data/goloom.db"),
		EncryptionKey:                    strings.TrimSpace(getEnv("ENCRYPTION_KEY", "")),
		AllowPrivateProviderInstanceURLs: parseBoolEnv("ALLOW_PRIVATE_PROVIDER_INSTANCE_URLS", false),
		AllowedOrigins:                   splitCSV(getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173")),
		RateLimitPerMinute:               getInt("RATE_LIMIT_PER_MINUTE", 120),
		RateLimitAuthenticatedPerMinute:  getInt("RATE_LIMIT_AUTHENTICATED_PER_MINUTE", 0),
		SchedulerPollInterval:               getDuration("SCHEDULER_POLL_INTERVAL", 15*time.Second),
		SchedulerMetricsSyncInterval:        getDuration("SCHEDULER_METRICS_SYNC_INTERVAL", 10*time.Minute),
		SchedulerAccountHealthInterval:      getDuration("SCHEDULER_ACCOUNT_HEALTH_INTERVAL", time.Hour),
		SchedulerExternalPostImportInterval: getDuration("SCHEDULER_EXTERNAL_POST_IMPORT_INTERVAL", 15*time.Minute),
		SchedulerRSSImportInterval:          getDuration("SCHEDULER_RSS_IMPORT_INTERVAL", 5*time.Minute),
		SchedulerWorkers:                    getInt("SCHEDULER_WORKERS", 4),
		LogLevel:                         strings.TrimSpace(getEnv("LOG_LEVEL", "")),
		LogFormat:                        strings.TrimSpace(getEnv("LOG_FORMAT", "")),
		OIDCIssuerURL:                    getEnv("OIDC_ISSUER_URL", ""),
		OIDCClientID:                     getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:                 getEnv("OIDC_CLIENT_SECRET", ""),
		OIDCRedirectURI:                  getEnv("OIDC_REDIRECT_URI", ""),
		SessionTTL:                       getDuration("SESSION_TTL", 720*time.Hour),
		BootstrapAdminEmail:              getEnv("BOOTSTRAP_ADMIN_EMAIL", "admin@localhost"),
		BootstrapAdminName:               getEnv("BOOTSTRAP_ADMIN_NAME", "Local Administrator"),
		BootstrapAdminToken:              getEnv("BOOTSTRAP_ADMIN_TOKEN", ""),
		BootstrapEnabled:                 parseBoolEnv("BOOTSTRAP_ENABLED", false),
		MastodonAppName:                  getEnv("MASTODON_APP_NAME", "goloom"),
		MastodonRedirectURI:              "",
		MastodonWebsite:                  getEnv("MASTODON_WEBSITE", ""),
		MastodonDefaultScopes:            splitCSV(getEnv("MASTODON_DEFAULT_SCOPES", "read,write")),
		MCPEnabled:                       parseBoolEnv("MCP_ENABLED", true),
		MCPRateLimitPerMinute:            getInt("MCP_RATE_LIMIT_PER_MINUTE", 60),
	}

	cfg.MastodonRedirectURI = getEnv("MASTODON_REDIRECT_URI", strings.TrimRight(cfg.PublicBaseURL, "/")+"/v1/oauth/mastodon/callback")

	if cfg.OIDCRedirectURI == "" {
		cfg.OIDCRedirectURI = strings.TrimRight(cfg.PublicBaseURL, "/") + "/v1/oauth/oidc/callback"
	}

	sumDev := sha256.Sum256([]byte("development-insecure-key"))
	devFallback := hex.EncodeToString(sumDev[:])

	isProd := strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")
	if cfg.EncryptionKey == "" {
		if isProd {
			return Config{}, fmt.Errorf("ENCRYPTION_KEY is required when APP_ENV is production")
		}
		cfg.EncryptionKey = devFallback
	}
	if isProd && cfg.EncryptionKey == devFallback {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY cannot use the development default when APP_ENV is production")
	}

	if len(cfg.EncryptionKey) < 32 {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY must be at least 32 characters")
	}

	if cfg.RateLimitPerMinute <= 0 {
		cfg.RateLimitPerMinute = 120
	}
	authCap := cfg.RateLimitAuthenticatedPerMinute
	if authCap <= 0 {
		authCap = cfg.RateLimitPerMinute * 5
		if authCap < 300 {
			authCap = 300
		}
	}
	cfg.RateLimitAuthenticatedPerMinute = authCap
	if cfg.RateLimitAuthenticatedPerMinute < cfg.RateLimitPerMinute {
		cfg.RateLimitAuthenticatedPerMinute = cfg.RateLimitPerMinute
	}

	return cfg, nil
}

// SlogLevel returns the effective slog level for LOG_LEVEL / APP_ENV.
func (c Config) SlogLevel() slog.Level {
	l := strings.ToLower(strings.Trim(c.LogLevel, "\" '"))
	switch l {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	case "":
		if strings.EqualFold(c.AppEnv, "production") {
			return slog.LevelInfo
		}
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// LogFormatJSON selects JSON logs (good for Docker log drivers / aggregators) vs text (easier local reading).
func (c Config) LogFormatJSON() bool {
	f := strings.ToLower(strings.Trim(c.LogFormat, "\" '"))
	switch f {
	case "json":
		return true
	case "text":
		return false
	case "":
		return strings.EqualFold(c.AppEnv, "production")
	default:
		return true
	}
}

// LogFormatName returns "json" or "text" for diagnostics.
func (c Config) LogFormatName() string {
	if c.LogFormatJSON() {
		return "json"
	}
	return "text"
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseBoolEnv(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func getInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	items := strings.Split(value, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
