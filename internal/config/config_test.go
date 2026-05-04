package config

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// 32+ chars; required when APP_ENV is production.
const testProductionEncryptionKey = "test-production-encryption-key-32b!"

func TestSlogLevel(t *testing.T) {
	tests := []struct {
		name     string
		appEnv   string
		logLevel string
		expected slog.Level
	}{
		{
			name:     "production default",
			appEnv:   "production",
			logLevel: "",
			expected: slog.LevelInfo,
		},
		{
			name:     "development default",
			appEnv:   "development",
			logLevel: "",
			expected: slog.LevelDebug,
		},
		{
			name:     "production override with debug",
			appEnv:   "production",
			logLevel: "debug",
			expected: slog.LevelDebug,
		},
		{
			name:     "production override with debug and whitespace",
			appEnv:   "production ",
			logLevel: " debug ",
			expected: slog.LevelDebug,
		},
		{
			name:     "production override with caps",
			appEnv:   "PRODUCTION",
			logLevel: "DEBUG",
			expected: slog.LevelDebug,
		},
		{
			name:     "production default with whitespace",
			appEnv:   " production",
			logLevel: "",
			expected: slog.LevelInfo,
		},
		{
			name:     "quoted log level",
			appEnv:   "production",
			logLevel: "\"debug\"",
			expected: slog.LevelDebug,
		},
		{
			name:     "production override with warn",
			appEnv:   "production",
			logLevel: "warn",
			expected: slog.LevelWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("APP_ENV", tt.appEnv)
			os.Setenv("LOG_LEVEL", tt.logLevel)
			defer os.Unsetenv("APP_ENV")
			defer os.Unsetenv("LOG_LEVEL")
			if strings.EqualFold(strings.TrimSpace(tt.appEnv), "production") {
				os.Setenv("ENCRYPTION_KEY", testProductionEncryptionKey)
				defer os.Unsetenv("ENCRYPTION_KEY")
			} else {
				os.Unsetenv("ENCRYPTION_KEY")
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("failed to load config: %v", err)
			}

			if got := cfg.SlogLevel(); got != tt.expected {
				t.Errorf("SlogLevel() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLogFormatJSON(t *testing.T) {
	tests := []struct {
		name      string
		appEnv    string
		logFormat string
		expected  bool
	}{
		{
			name:      "production default",
			appEnv:    "production",
			logFormat: "",
			expected:  true,
		},
		{
			name:      "development default",
			appEnv:    "development",
			logFormat: "",
			expected:  false,
		},
		{
			name:      "explicit json",
			appEnv:    "development",
			logFormat: "json",
			expected:  true,
		},
		{
			name:      "explicit text",
			appEnv:    "production",
			logFormat: "text",
			expected:  false,
		},
		{
			name:      "quoted json",
			appEnv:    "development",
			logFormat: "'json'",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("APP_ENV", tt.appEnv)
			os.Setenv("LOG_FORMAT", tt.logFormat)
			defer os.Unsetenv("APP_ENV")
			defer os.Unsetenv("LOG_FORMAT")
			if strings.EqualFold(strings.TrimSpace(tt.appEnv), "production") {
				os.Setenv("ENCRYPTION_KEY", testProductionEncryptionKey)
				defer os.Unsetenv("ENCRYPTION_KEY")
			} else {
				os.Unsetenv("ENCRYPTION_KEY")
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("failed to load config: %v", err)
			}

			if got := cfg.LogFormatJSON(); got != tt.expected {
				t.Errorf("LogFormatJSON() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLoad_productionRequiresEncryptionKey(t *testing.T) {
	os.Setenv("APP_ENV", "production")
	os.Unsetenv("ENCRYPTION_KEY")
	defer os.Unsetenv("APP_ENV")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when ENCRYPTION_KEY is missing in production")
	}
	if !strings.Contains(err.Error(), "ENCRYPTION_KEY") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_productionRejectsDevelopmentDefaultKey(t *testing.T) {
	os.Setenv("APP_ENV", "production")
	defer os.Unsetenv("APP_ENV")

	sum := sha256.Sum256([]byte("development-insecure-key"))
	os.Setenv("ENCRYPTION_KEY", hex.EncodeToString(sum[:]))
	defer os.Unsetenv("ENCRYPTION_KEY")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when ENCRYPTION_KEY is the development default in production")
	}
}
