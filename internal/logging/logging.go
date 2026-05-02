package logging

import (
	"log/slog"
	"os"

	"git.f4mily.net/goloom/internal/config"
)

// New returns the application logger writing to stdout (Docker captures this as container logs).
func New(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: cfg.SlogLevel(),
	}
	var h slog.Handler
	if cfg.LogFormatJSON() {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}
