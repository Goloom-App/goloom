package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
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

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

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

	apiHandler := api.New(dataStore, authService, providers, cfg)
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

	logger.Info("starting server", "addr", cfg.HTTPAddr, "env", cfg.AppEnv)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
