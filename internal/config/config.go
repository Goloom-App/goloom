package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                string
	HTTPAddr              string
	DatabaseURL           string
	EncryptionKey         string
	AllowedOrigins        []string
	RateLimitPerMinute    int
	SchedulerPollInterval time.Duration
	SchedulerWorkers      int

	OIDCIssuerURL    string
	OIDCClientID     string
	OIDCClientSecret string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:                getEnv("APP_ENV", "development"),
		HTTPAddr:              getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:           getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/goloom?sslmode=disable"),
		EncryptionKey:         getEnv("ENCRYPTION_KEY", ""),
		AllowedOrigins:        splitCSV(getEnv("ALLOWED_ORIGINS", "http://localhost:3000")),
		RateLimitPerMinute:    getInt("RATE_LIMIT_PER_MINUTE", 60),
		SchedulerPollInterval: getDuration("SCHEDULER_POLL_INTERVAL", 15*time.Second),
		SchedulerWorkers:      getInt("SCHEDULER_WORKERS", 4),
		OIDCIssuerURL:         getEnv("OIDC_ISSUER_URL", ""),
		OIDCClientID:          getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:      getEnv("OIDC_CLIENT_SECRET", ""),
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
