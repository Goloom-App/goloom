package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store/postgres"
)

type Service struct {
	logger       *slog.Logger
	store        *postgres.Store
	providers    *provider.Registry
	pollInterval time.Duration
	workers      int
}

func New(logger *slog.Logger, store *postgres.Store, providers *provider.Registry, pollInterval time.Duration, workers int) *Service {
	if workers <= 0 {
		workers = 1
	}
	return &Service{
		logger:       logger,
		store:        store,
		providers:    providers,
		pollInterval: pollInterval,
		workers:      workers,
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
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		token, err := s.store.DecryptAccessToken(account)
		if err != nil {
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		result, err := providerImpl.Publish(ctx, account, token, provider.PublishRequest{Content: post.Content})
		if err != nil {
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		if err := s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusPosted, result.URL, ""); err != nil {
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
