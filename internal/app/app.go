package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/logging"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/scheduler"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
	"git.f4mily.net/goloom/internal/webui"
)

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg)
	level := cfg.SlogLevel()
	logger.Info("goloom starting",
		"app_env", cfg.AppEnv,
		"log_level", level.String(),
		"log_format", cfg.LogFormatName(),
		"http_addr", cfg.HTTPAddr,
		"public_base_url", cfg.PublicBaseURL,
		"database_backend", databaseBackend(cfg.DatabaseURL),
		"scheduler_poll_interval", cfg.SchedulerPollInterval.String(),
		"scheduler_workers", cfg.SchedulerWorkers,
		"rate_limit_per_minute", cfg.RateLimitPerMinute,
		"oidc_enabled", cfg.OIDCIssuerURL != "" && cfg.OIDCClientID != "",
		"bootstrap_admin_configured", cfg.BootstrapAdminToken != "",
	)

	encrypter, err := security.NewEncrypter(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("build encrypter: %w", err)
	}

	dataStore, err := store.Open(ctx, cfg.DatabaseURL, encrypter)
	if err != nil {
		return err
	}
	defer dataStore.Close()

	if cfg.BootstrapAdminToken != "" {
		if err := dataStore.EnsureBootstrapAdmin(ctx, cfg.BootstrapAdminEmail, cfg.BootstrapAdminName, cfg.BootstrapAdminToken); err != nil {
			return fmt.Errorf("bootstrap admin: %w", err)
		}
	}

	if err := dataStore.EnsurePersonalTeamsMigrated(ctx); err != nil {
		return fmt.Errorf("personal workspace migration: %w", err)
	}

	authService, err := auth.New(ctx, cfg, dataStore)
	if err != nil {
		return fmt.Errorf("build auth service: %w", err)
	}

	providers := provider.NewRegistry(
		provider.NewBlueskyProvider(),
		provider.NewFriendicaProvider(),
		provider.NewMastodonProvider(provider.MastodonRegistrationConfig{
			AppName:       cfg.MastodonAppName,
			RedirectURI:   cfg.MastodonRedirectURI,
			Website:       cfg.MastodonWebsite,
			DefaultScopes: cfg.MastodonDefaultScopes,
		}),
	)

	schedulerService := scheduler.New(
		logger,
		dataStore,
		providers,
		cfg.SchedulerPollInterval,
		cfg.SchedulerWorkers,
	)
	go schedulerService.Start(ctx)

	apiHandler := api.New(logger, dataStore, authService, providers, cfg)
	apiRoot := apiHandler.Handler(security.NewLimiter(cfg.RateLimitPerMinute), cfg.AllowedOrigins)
	rootHandler := http.NewServeMux()
	rootHandler.Handle("/healthz", apiRoot)
	rootHandler.Handle("/v1/", apiRoot)
	rootHandler.Handle("/", webui.Handler())
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           rootHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("http server listening", "addr", cfg.HTTPAddr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, stopping http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Warn("http server shutdown error", "error", err)
			return err
		}
		logger.Info("http server stopped")
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func databaseBackend(raw string) string {
	u := strings.TrimSpace(strings.ToLower(raw))
	if strings.HasPrefix(u, "postgres://") || strings.HasPrefix(u, "postgresql://") {
		return "postgres"
	}
	return "sqlite"
}
