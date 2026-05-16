package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/scheduling"
	"git.f4mily.net/goloom/internal/socialtokens"
	"git.f4mily.net/goloom/internal/store"
)

type Service struct {
	logger                *slog.Logger
	store                 store.Store
	providers             *provider.Registry
	pollInterval          time.Duration
	metricSyncInterval    time.Duration
	accountHealthInterval time.Duration
	workers               int
}

func New(logger *slog.Logger, store store.Store, providers *provider.Registry, pollInterval time.Duration, workers int, metricSyncInterval time.Duration, accountHealthInterval time.Duration) *Service {
	if workers <= 0 {
		workers = 1
	}
	return &Service{
		logger:                logger,
		store:                 store,
		providers:             providers,
		pollInterval:          pollInterval,
		metricSyncInterval:    metricSyncInterval,
		accountHealthInterval: accountHealthInterval,
		workers:               workers,
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

	if s.accountHealthInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runAccountHealthLoop(ctx)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.runAccountMetricsLoop(ctx)
	}()

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	defer close(queue)
	defer wg.Wait()

	for {
		if err := s.materializePostTemplates(ctx); err != nil {
			s.logger.Error("post template materialize failed", "error", err)
		}
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

func (s *Service) runAccountMetricsLoop(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		if s.tryLock(ctx, "account_metrics", 23*time.Hour) {
			s.accountMetricsJob(ctx)
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

	var versionContentByAccount map[string]string
	if len(accounts) > 0 {
		vers, err := s.store.ListPostVersionsForTeamPost(ctx, post.TeamID, post.ID)
		if err != nil {
			s.failPost(ctx, post, fmt.Errorf("list post versions: %w", err))
			return
		}
		for _, v := range vers {
			if c := strings.TrimSpace(v.Content); c != "" {
				if versionContentByAccount == nil {
					versionContentByAccount = make(map[string]string)
				}
				versionContentByAccount[v.AccountID] = c
			}
		}
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

		content := post.Content
		if versionContentByAccount != nil {
			if o, ok := versionContentByAccount[account.ID]; ok {
				content = o
			}
		}
		publishedAt := time.Now().UTC()
		content = domain.ExpandDynamicVariables(content, publishedAt, post.TemplateCounter)

		localMedia := domain.FilterMediaIDsForAccount(post.MediaIDs, post.MediaExcludeByAccount, account.ID)
		remoteMedia, err := s.syncMediaToProvider(ctx, post.TeamID, localMedia, account, providerImpl, provider.PublishAuth{
			AccessToken:  token,
			RefreshToken: refreshToken,
		})
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
			Content:     content,
			MediaIDs:    remoteMedia,
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
		if s.tryLock(ctx, "metric_sync", s.metricSyncInterval-1*time.Minute) {
			s.fetchMetricsJob(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runAccountHealthLoop periodically logs OAuth accounts whose access token expiry is in the past
// or within the next 48 hours (re-auth / refresh needed).
func (s *Service) runAccountHealthLoop(ctx context.Context) {
	ticker := time.NewTicker(s.accountHealthInterval)
	defer ticker.Stop()
	for {
		if s.tryLock(ctx, "account_health", s.accountHealthInterval-1*time.Minute) {
			s.accountHealthJob(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) accountHealthJob(ctx context.Context) {
	horizon := time.Now().UTC().Add(48 * time.Hour)
	accounts, err := s.store.ListOAuthAccountsWithAccessTokenExpiringBefore(ctx, horizon, 500)
	if err != nil {
		s.logger.Error("account health list failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for _, a := range accounts {
		exp := a.AccessTokenExpiresAt.UTC()
		if exp.Before(now) {
			s.logger.Warn("oauth access token expired",
				"account_id", a.ID,
				"team_id", a.TeamID,
				"provider", a.Provider,
				"username", a.Username,
				"access_token_expires_at", exp.Format(time.RFC3339),
			)
			continue
		}
		s.logger.Info("oauth access token expiring soon",
			"account_id", a.ID,
			"team_id", a.TeamID,
			"provider", a.Provider,
			"username", a.Username,
			"access_token_expires_at", exp.Format(time.RFC3339),
			"hours_remaining", time.Until(exp).Hours(),
		)
	}
}

func (s *Service) accountMetricsJob(ctx context.Context) {
	accounts, err := s.store.ListAccountsForMetricsSync(ctx, 2000)
	if err != nil {
		s.logger.Error("account metrics list failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for _, account := range accounts {
		providerImpl, ok := s.providers.Get(account.Provider)
		if !ok {
			continue
		}
		accFresh, err := socialtokens.EnsureMastodonFresh(ctx, s.store, s.providers, account)
		if err == nil {
			account = accFresh
		}
		token, err := s.store.DecryptAccessToken(account)
		if err != nil {
			s.logger.Warn("account metrics decrypt access failed", "account_id", account.ID, "error", err)
			continue
		}
		refreshToken, err := s.store.DecryptRefreshToken(account)
		if err != nil {
			s.logger.Warn("account metrics decrypt refresh failed", "account_id", account.ID, "error", err)
			continue
		}
		metrics, err := providerImpl.GetAccountMetrics(ctx, account, provider.PublishAuth{
			AccessToken:  token,
			RefreshToken: refreshToken,
		})
		if err != nil {
			s.logger.Warn("account metrics fetch failed", "account_id", account.ID, "provider", account.Provider, "error", err)
			continue
		}
		snapshot := make(map[string]int64, len(metrics))
		for _, metric := range metrics {
			name := strings.TrimSpace(metric.Name)
			if name == "" {
				continue
			}
			snapshot[name] = metric.Value
		}
		if err := s.store.UpsertAccountMetrics(ctx, account.ID, snapshot, now); err != nil {
			s.logger.Warn("account metrics upsert failed", "account_id", account.ID, "error", err)
		}
	}
}

// SyncPostMetricsNow pulls engagement metrics for eligible posted targets (admin trigger; bypasses job lock).
func (s *Service) SyncPostMetricsNow(ctx context.Context) {
	s.fetchMetricsJob(ctx)
}

// SyncAccountMetricsNow refreshes follower counts for all connected accounts (admin trigger).
func (s *Service) SyncAccountMetricsNow(ctx context.Context) {
	s.accountMetricsJob(ctx)
}

// fetchMetricsJob pulls provider engagement metrics for posted targets due for refresh.
func (s *Service) fetchMetricsJob(ctx context.Context) {
	since := time.Now().Add(-30 * 24 * time.Hour)
	utcDay := time.Now().UTC().Format("2006-01-02")
	rows, err := s.store.ListPostedTargetsForMetricSync(ctx, since, utcDay, 500)
	if err != nil {
		s.logger.Error("metric sync list failed", "error", err)
		return
	}
	for _, row := range rows {
		s.syncOnePostTargetMetrics(ctx, row, utcDay)
	}
}

func (s *Service) syncOnePostTargetMetrics(ctx context.Context, row domain.PostedTargetForMetricSync, utcDay string) {
	pImpl, ok := s.providers.Get(row.Account.Provider)
	if !ok {
		return
	}
	account := row.Account
	accFresh, err := socialtokens.EnsureMastodonFresh(ctx, s.store, s.providers, account)
	if err == nil {
		account = accFresh
	}
	token, err := s.store.DecryptAccessToken(account)
	if err != nil {
		s.logger.Warn("metric sync decrypt access failed", "post_id", row.PostID, "account_id", account.ID, "error", err)
		return
	}
	refreshToken, err := s.store.DecryptRefreshToken(account)
	if err != nil {
		s.logger.Warn("metric sync decrypt refresh failed", "post_id", row.PostID, "account_id", account.ID, "error", err)
		return
	}
	metricsURL := provider.MetricsPublishedURL(account, row.PublishedURL, row.PublishMetadata)
	readings, err := pImpl.GetMetrics(ctx, account, provider.PublishAuth{
		AccessToken:  token,
		RefreshToken: refreshToken,
	}, metricsURL)
	if err != nil {
		s.logger.Warn("metric sync fetch failed", "post_id", row.PostID, "account_id", account.ID, "provider", account.Provider, "published_url", metricsURL, "error", err)
		return
	}
	if len(readings) == 0 {
		s.logger.Warn("metric sync returned no readings", "post_id", row.PostID, "account_id", account.ID, "provider", account.Provider)
		return
	}
	m := make(map[string]int64, len(readings))
	for _, x := range readings {
		name := strings.TrimSpace(x.Name)
		if name == "" {
			continue
		}
		m[name] = x.Value
	}
	if len(m) == 0 {
		return
	}
	if err := s.store.UpsertPostMetrics(ctx, row.PostID, account.ID, m); err != nil {
		s.logger.Warn("metric sync upsert failed", "post_id", row.PostID, "account_id", account.ID, "error", err)
		return
	}
	if err := s.store.MarkScheduledPostTargetMetricsSynced(ctx, row.PostID, account.ID, utcDay); err != nil {
		s.logger.Warn("metric sync mark cursor failed", "post_id", row.PostID, "account_id", account.ID, "error", err)
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

func (s *Service) syncMediaToProvider(ctx context.Context, teamID string, localMediaIDs []string, account domain.SocialAccount, p provider.SocialMediaProvider, auth provider.PublishAuth) ([]string, error) {
	if len(localMediaIDs) == 0 {
		return nil, nil
	}

	remoteIDs := make([]string, 0, len(localMediaIDs))
	for _, id := range localMediaIDs {
		// 1. Check if it's already a remote ID (e.g. legacy or direct upload)
		item, err := s.store.GetMediaItemByID(ctx, teamID, id)
		if err != nil {
			// Not a local UUID or not found, assume it's already a remote ID
			remoteIDs = append(remoteIDs, id)
			continue
		}

		// 2. Check if we already have a mapping for this account
		mapping, err := s.store.GetMediaProviderMapping(ctx, item.ID, account.ID)
		if err == nil {
			// Found mapping, use it
			remoteIDs = append(remoteIDs, mapping.RemoteID)
			continue
		}

		// 3. Not mapped, upload now
		filePath := store.GetMediaFilePath(teamID, item.Sha256)
		file, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("open local media %q: %w", item.ID, err)
		}
		// Uploading to provider.
		remoteID, err := p.UploadMedia(ctx, account, auth, file, item.Filename, item.MimeType, "")
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("provider upload %q: %w", item.ID, err)
		}

		// 4. Cache the mapping
		_ = s.store.UpsertMediaProviderMapping(ctx, domain.MediaProviderMapping{
			MediaID:   item.ID,
			AccountID: account.ID,
			RemoteID:  remoteID,
		})

		remoteIDs = append(remoteIDs, remoteID)
	}

	return remoteIDs, nil
}

func (s *Service) tryLock(ctx context.Context, lockID string, duration time.Duration) bool {
	locked, err := s.store.TryAcquireLock(ctx, lockID, duration)
	if err != nil {
		s.logger.Error("failed to acquire lock", "lock_id", lockID, "error", err)
		return false
	}
	return locked
}

func (s *Service) materializePostTemplates(ctx context.Context) error {
	templates, err := s.store.ListDuePostTemplates(ctx, 100)
	if err != nil {
		return err
	}
	for _, tmpl := range templates {
		occ := tmpl.NextMaterializeAt
		if occ == nil {
			continue
		}
		rule, err := scheduling.ParseRecurrenceJSON(tmpl.RecurrenceJSON)
		if err != nil {
			s.logger.Warn("invalid template recurrence_json", "template_id", tmpl.ID, "error", err)
			continue
		}
		skipped, err := s.store.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, *occ)
		if err != nil {
			return err
		}
		nextOcc, err := scheduling.NextOccurrence(rule, *occ)
		if err != nil {
			s.logger.Warn("template next occurrence failed", "template_id", tmpl.ID, "error", err)
			continue
		}
		if skipped {
			if err := s.store.AdvancePostTemplateSchedule(ctx, tmpl.ID, &nextOcc, tmpl.CounterNext); err != nil {
				return err
			}
			continue
		}
		authorID := tmpl.AuthorUserID
		tplID := tmpl.ID
		counterVal := tmpl.CounterNext
		input := domain.CreatePostInput{
			Title:                 tmpl.Title,
			Content:               tmpl.Content,
			ScheduledAt:           *occ,
			TargetAccounts:        tmpl.TargetAccountIDs,
			Visibility:            tmpl.Visibility,
			MediaIDs:              tmpl.MediaIDs,
			MediaExcludeByAccount: tmpl.MediaExcludeByAccount,
			Draft:                 false,
			AuthorUserID:          &authorID,
			PostTemplateID:        &tplID,
			TemplateCounter:       &counterVal,
		}
		principal := domain.AuthenticatedPrincipal{User: domain.User{ID: tmpl.AuthorUserID}}
		if _, err := s.store.CreateScheduledPost(ctx, tmpl.TeamID, principal, input); err != nil {
			s.logger.Error("materialize scheduled post from template failed", "template_id", tmpl.ID, "error", err)
			continue
		}
		if err := s.store.AdvancePostTemplateSchedule(ctx, tmpl.ID, &nextOcc, tmpl.CounterNext+1); err != nil {
			return err
		}
	}
	return nil
}
