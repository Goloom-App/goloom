package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/socialtokens"
	"git.f4mily.net/goloom/internal/store"
)

type Service struct {
	logger               *slog.Logger
	store                store.Store
	providers            *provider.Registry
	pollInterval         time.Duration
	metricSyncInterval   time.Duration
	workers              int
}

func New(logger *slog.Logger, store store.Store, providers *provider.Registry, pollInterval time.Duration, workers int, metricSyncInterval time.Duration) *Service {
	if workers <= 0 {
		workers = 1
	}
	return &Service{
		logger:             logger,
		store:              store,
		providers:          providers,
		pollInterval:       pollInterval,
		metricSyncInterval: metricSyncInterval,
		workers:            workers,
	}
}

func (s *Service) Start(ctx context.Context) {
	queue := make(chan domain.ScheduledPost, s.workers*2)
	var wg sync.WaitGroup

	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case post, ok := <-queue:
					if !ok {
						return
					}
					s.processPost(ctx, post)
				}
			}
		}()
	}

	if s.metricSyncInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runMetricSyncLoop(ctx)
		}()
	}

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	defer close(queue)
	defer wg.Wait()

	for {
		if err := s.enqueueDuePosts(ctx, queue); err != nil {
			s.logger.Error("scheduler poll failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) enqueueDuePosts(ctx context.Context, queue chan<- domain.ScheduledPost) error {
	posts, err := s.store.ListDuePosts(ctx, s.workers*4)
	if err != nil {
		return err
	}

	for _, post := range posts {
		if err := s.store.MarkPostProcessing(ctx, post.ID); err != nil {
			s.logger.Warn("failed to mark post processing", "post_id", post.ID, "error", err)
			continue
		}
		queue <- post
	}
	return nil
}

func (s *Service) processPost(ctx context.Context, post domain.ScheduledPost) {
	accounts, err := s.store.LoadPostTargets(ctx, post.ID)
	if err != nil {
		s.failPost(ctx, post, fmt.Errorf("load targets: %w", err))
		return
	}

	var firstErr error
	for _, account := range accounts {
		providerImpl, ok := s.providers.Get(account.Provider)
		if !ok {
			err := fmt.Errorf("unsupported provider %q", account.Provider)
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		acc, err := socialtokens.EnsureMastodonFresh(ctx, s.store, s.providers, account)
		if err != nil {
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		account = acc

		token, err := s.store.DecryptAccessToken(account)
		if err != nil {
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		refreshToken, err := s.store.DecryptRefreshToken(account)
		if err != nil {
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		result, err := providerImpl.Publish(ctx, account, provider.PublishAuth{
			AccessToken:  token,
			RefreshToken: refreshToken,
		}, provider.PublishRequest{
			Content:     post.Content,
			MediaIDs:    post.MediaIDs,
			Visibility:  post.Visibility,
			ScheduledAt: nil,
		})
		if err != nil {
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		if err := s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusPosted, result.URL, "", result.Metadata); err != nil {
			s.logger.Warn("failed to mark post target result", "post_id", post.ID, "account_id", account.ID, "error", err)
		}
	}

	if firstErr != nil {
		s.failPost(ctx, post, firstErr)
		return
	}

	if err := s.store.MarkPostResult(ctx, post.ID, post.AttemptCount+1, domain.PostStatusPosted, "", nil); err != nil {
		s.logger.Error("failed to mark post as posted", "post_id", post.ID, "error", err)
	}
}

func (s *Service) runMetricSyncLoop(ctx context.Context) {
	ticker := time.NewTicker(s.metricSyncInterval)
	defer ticker.Stop()
	for {
		s.syncPostedMetrics(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) syncPostedMetrics(ctx context.Context) {
	since := time.Now().Add(-30 * 24 * time.Hour)
	rows, err := s.store.ListPostedTargetsForMetricSync(ctx, since, 500)
	if err != nil {
		s.logger.Error("metric sync list failed", "error", err)
		return
	}
	for _, row := range rows {
		pImpl, ok := s.providers.Get(row.Account.Provider)
		if !ok {
			continue
		}
		token, err := s.store.DecryptAccessToken(row.Account)
		if err != nil {
			s.logger.Warn("metric sync decrypt access failed", "post_id", row.PostID, "account_id", row.Account.ID, "error", err)
			continue
		}
		refreshToken, err := s.store.DecryptRefreshToken(row.Account)
		if err != nil {
			s.logger.Warn("metric sync decrypt refresh failed", "post_id", row.PostID, "account_id", row.Account.ID, "error", err)
			continue
		}
		readings, err := pImpl.GetMetrics(ctx, row.Account, provider.PublishAuth{
			AccessToken:  token,
			RefreshToken: refreshToken,
		}, row.PublishedURL)
		if err != nil {
			s.logger.Warn("metric sync fetch failed", "post_id", row.PostID, "account_id", row.Account.ID, "provider", row.Account.Provider, "error", err)
			continue
		}
		m := make(map[string]int64, len(readings))
		for _, x := range readings {
			name := strings.TrimSpace(x.Name)
			if name == "" {
				continue
			}
			m[name] = x.Value
		}
		if err := s.store.UpsertPostMetrics(ctx, row.PostID, row.Account.ID, m); err != nil {
			s.logger.Warn("metric sync upsert failed", "post_id", row.PostID, "account_id", row.Account.ID, "error", err)
		}
	}
}

func (s *Service) failPost(ctx context.Context, post domain.ScheduledPost, err error) {
	attemptCount := post.AttemptCount + 1
	if attemptCount >= 5 {
		if markErr := s.store.MarkPostResult(ctx, post.ID, attemptCount, domain.PostStatusFailed, err.Error(), nil); markErr != nil {
			s.logger.Error("failed to mark post failed", "post_id", post.ID, "error", markErr)
		}
		return
	}

	nextAttempt := time.Now().Add(time.Duration(attemptCount*attemptCount) * time.Minute)
	if markErr := s.store.MarkPostResult(ctx, post.ID, attemptCount, domain.PostStatusFailed, err.Error(), &nextAttempt); markErr != nil {
		s.logger.Error("failed to schedule retry", "post_id", post.ID, "error", markErr)
	}
}
