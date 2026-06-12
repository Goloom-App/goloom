package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/logging"
	"git.f4mily.net/goloom/internal/mcp"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/scheduler"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/sse"
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
		"scheduler_metrics_sync_interval", cfg.SchedulerMetricsSyncInterval.String(),
		"scheduler_account_health_interval", cfg.SchedulerAccountHealthInterval.String(),
		"scheduler_external_post_import_interval", cfg.SchedulerExternalPostImportInterval.String(),
		"scheduler_rss_import_interval", cfg.SchedulerRSSImportInterval.String(),
		"scheduler_workers", cfg.SchedulerWorkers,
		"rate_limit_per_minute", cfg.RateLimitPerMinute,
		"rate_limit_authenticated_per_minute", cfg.RateLimitAuthenticatedPerMinute,
		"oidc_enabled", cfg.OIDCIssuerURL != "" && cfg.OIDCClientID != "",
		"bootstrap_recovery_configured", cfg.BootstrapEnabled && cfg.BootstrapAdminToken != "",
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

	// Wrap logger with DB-backed persistence so scheduler and API logs
	// are captured in the log_entries table (visible in the admin UI).
	dbLogHandler := logging.NewDBHandler(logger.Handler(), dataStore)
	logger = slog.New(dbLogHandler)
	defer dbLogHandler.Close()

	users, err := dataStore.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	bootstrapToken := strings.TrimSpace(cfg.BootstrapAdminToken)
	if len(users) == 0 {
		if bootstrapToken == "" {
			generated, genErr := randomBootstrapToken()
			if genErr != nil {
				return fmt.Errorf("bootstrap token: %w", genErr)
			}
			bootstrapToken = generated
			fmt.Fprintln(os.Stdout, "")
			fmt.Fprintln(os.Stdout, "=== GOLOOM: first administrator sign-in (copy token below) ===")
			fmt.Fprintln(os.Stdout, bootstrapToken)
			fmt.Fprintln(os.Stdout, "================================================================")
			logger.Warn("database has no users: printed one-time bootstrap token to stdout (token value is not written to structured logs)")
		}
		if err := dataStore.EnsureBootstrapAdmin(ctx, cfg.BootstrapAdminEmail, cfg.BootstrapAdminName, bootstrapToken); err != nil {
			return fmt.Errorf("bootstrap admin: %w", err)
		}
	} else if bootstrapToken != "" {
		if err := dataStore.EnsureBootstrapAdmin(ctx, cfg.BootstrapAdminEmail, cfg.BootstrapAdminName, bootstrapToken); err != nil {
			return fmt.Errorf("bootstrap admin: %w", err)
		}
	}

	if err := dataStore.EnsurePersonalTeamsMigrated(ctx); err != nil {
		return fmt.Errorf("personal workspace migration: %w", err)
	}

	if err := dataStore.BackfillPostHashtags(ctx); err != nil {
		logger.Warn("hashtag backfill failed", "error", err)
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

	jobManager := aijobs.NewManager(dataStore, nil)

	schedulerService := scheduler.New(
		logger,
		dataStore,
		providers,
		cfg.SchedulerPollInterval,
		cfg.SchedulerWorkers,
		cfg.SchedulerMetricsSyncInterval,
		cfg.SchedulerAccountHealthInterval,
		cfg.SchedulerExternalPostImportInterval,
		cfg.SchedulerRSSImportInterval,
		jobManager,
	)
	go schedulerService.Start(ctx)

	catalog, err := i18n.Load()
	if err != nil {
		return fmt.Errorf("load i18n catalog: %w", err)
	}

	sseHub := sse.NewHub()
	defer sseHub.Close()
	apiHandler := api.New(logger, dataStore, authService, providers, cfg, schedulerService, catalog, jobManager, sseHub)
	apiChain := apiHandler.Handler(security.NewLimiter(cfg.RateLimitPerMinute, cfg.RateLimitAuthenticatedPerMinute), cfg.AllowedOrigins)
	rootHandler := http.NewServeMux()
	rootHandler.Handle("/healthz", apiChain)
	rootHandler.Handle("/v1/", apiChain)
	// External clients often use /api/v1/... ; routes are registered as /v1/... on the inner mux.
	rootHandler.Handle("/api/v1/", http.StripPrefix("/api", apiChain))

	// MCP server (optional, enabled by default)
	if cfg.MCPEnabled {
		mcpHandler := mcp.NewHandler(logger, dataStore, authService, providers, cfg)
		mcpLimiter := security.NewLimiter(cfg.MCPRateLimitPerMinute, cfg.MCPRateLimitPerMinute*3)
		mcpChain := security.CORSMiddleware(cfg.AllowedOrigins)(mcpLimiter.Middleware(mcpHandler))
		rootHandler.Handle("/mcp/", http.StripPrefix("/mcp", mcpChain))
		logger.Info("mcp server enabled", "rate_limit_per_minute", cfg.MCPRateLimitPerMinute)
	}

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

func randomBootstrapToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "gloom_bootstrap_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func databaseBackend(raw string) string {
	u := strings.TrimSpace(strings.ToLower(raw))
	if strings.HasPrefix(u, "postgres://") || strings.HasPrefix(u, "postgresql://") {
		return "postgres"
	}
	return "sqlite"
}
