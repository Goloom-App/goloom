package config

import (
	"log/slog"
	"os"
	"testing"
)

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
