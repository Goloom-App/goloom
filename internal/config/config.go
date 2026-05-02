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
	AppEnv                string
	HTTPAddr              string
	PublicBaseURL         string
	DatabaseURL           string
	EncryptionKey         string
	AllowedOrigins        []string
	RateLimitPerMinute    int
	SchedulerPollInterval time.Duration
	SchedulerWorkers      int

	// LogLevel: debug, info, warn, error — empty means derive from AppEnv (development=debug, production=info).
	LogLevel string
	// LogFormat: json (machine-friendly, default in production) or text (human-friendly, default in development).
	LogFormat string

	OIDCIssuerURL    string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURI  string

	BootstrapAdminEmail string
	BootstrapAdminName  string
	BootstrapAdminToken string

	MastodonAppName       string
	MastodonRedirectURI   string
	MastodonWebsite       string
	MastodonDefaultScopes []string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:                getEnv("APP_ENV", "development"),
		HTTPAddr:              getEnv("HTTP_ADDR", ":8080"),
		PublicBaseURL:         getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),
		DatabaseURL:           getEnv("DATABASE_URL", "file:./data/goloom.db"),
		EncryptionKey:         getEnv("ENCRYPTION_KEY", ""),
		AllowedOrigins:        splitCSV(getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173")),
		RateLimitPerMinute:    getInt("RATE_LIMIT_PER_MINUTE", 60),
		SchedulerPollInterval: getDuration("SCHEDULER_POLL_INTERVAL", 15*time.Second),
		SchedulerWorkers:      getInt("SCHEDULER_WORKERS", 4),
		LogLevel:              getEnv("LOG_LEVEL", ""),
		LogFormat:             getEnv("LOG_FORMAT", ""),
		OIDCIssuerURL:         getEnv("OIDC_ISSUER_URL", ""),
		OIDCClientID:          getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:      getEnv("OIDC_CLIENT_SECRET", ""),
		OIDCRedirectURI:       getEnv("OIDC_REDIRECT_URI", ""),
		BootstrapAdminEmail:   getEnv("BOOTSTRAP_ADMIN_EMAIL", "admin@localhost"),
		BootstrapAdminName:    getEnv("BOOTSTRAP_ADMIN_NAME", "Local Administrator"),
		BootstrapAdminToken:   getEnv("BOOTSTRAP_ADMIN_TOKEN", ""),
		MastodonAppName:       getEnv("MASTODON_APP_NAME", "goloom"),
		MastodonRedirectURI:   "",
		MastodonWebsite:       getEnv("MASTODON_WEBSITE", ""),
		MastodonDefaultScopes: splitCSV(getEnv("MASTODON_DEFAULT_SCOPES", "read,write")),
	}

	cfg.MastodonRedirectURI = getEnv("MASTODON_REDIRECT_URI", strings.TrimRight(cfg.PublicBaseURL, "/")+"/v1/oauth/mastodon/callback")

	if cfg.OIDCRedirectURI == "" {
		cfg.OIDCRedirectURI = strings.TrimRight(cfg.PublicBaseURL, "/") + "/v1/oauth/oidc/callback"
	}

	if cfg.EncryptionKey == "" {
		sum := sha256.Sum256([]byte("development-insecure-key"))
		cfg.EncryptionKey = hex.EncodeToString(sum[:])
	}

	if len(cfg.EncryptionKey) < 32 {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY must be at least 32 characters")
	}

	return cfg, nil
}

// SlogLevel returns the effective slog level for LOG_LEVEL / APP_ENV.
func (c Config) SlogLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(c.LogLevel)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
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
	switch strings.ToLower(strings.TrimSpace(c.LogFormat)) {
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
