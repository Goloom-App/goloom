package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/socialtokens"
	"git.f4mily.net/goloom/internal/store"
)

type Service struct {
	logger                     *slog.Logger
	store                      store.Store
	providers                  *provider.Registry
	jobManager                 *aijobs.Manager
	pollInterval               time.Duration
	metricSyncInterval         time.Duration
	accountHealthInterval      time.Duration
	externalPostImportInterval time.Duration
	rssImportInterval          time.Duration
	workers                    int

	accountMetricsMu      sync.Mutex
	postMetricsMu         sync.Mutex
	accountHealthMu       sync.Mutex
	externalPostImportMu  sync.Mutex
	rssImportMu           sync.Mutex
}

func New(logger *slog.Logger, store store.Store, providers *provider.Registry, pollInterval time.Duration, workers int, metricSyncInterval time.Duration, accountHealthInterval time.Duration, externalPostImportInterval time.Duration, rssImportInterval time.Duration, jobManager *aijobs.Manager) *Service {
	if workers <= 0 {
		workers = 1
	}
	return &Service{
		logger:                     logger,
		store:                      store,
		providers:                  providers,
		jobManager:                 jobManager,
		pollInterval:               pollInterval,
		metricSyncInterval:         metricSyncInterval,
		accountHealthInterval:      accountHealthInterval,
		externalPostImportInterval: externalPostImportInterval,
		rssImportInterval:          rssImportInterval,
		workers:                    workers,
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

	if s.externalPostImportInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runExternalPostImportLoop(ctx)
		}()
	}

	if s.rssImportInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runRSSImportLoop(ctx)
		}()
	}

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
	s.accountMetricsJob(ctx)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.accountMetricsMu.Lock()
			s.accountMetricsJob(ctx)
			s.accountMetricsMu.Unlock()
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
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil, "")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		acc, err := socialtokens.EnsureMastodonFresh(ctx, s.store, s.providers, account)
		if err != nil {
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil, "")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		account = acc

		token, err := s.store.DecryptAccessToken(account)
		if err != nil {
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil, "")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		refreshToken, err := s.store.DecryptRefreshToken(account)
		if err != nil {
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil, "")
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
		content = domain.ExpandDynamicVariables(content, publishedAt, post.TemplateCounter, nil)

		localMedia := domain.FilterMediaIDsForAccount(post.MediaIDs, post.MediaExcludeByAccount, account.ID)
		remoteMedia, err := s.syncMediaToProvider(ctx, post.TeamID, localMedia, account, providerImpl, provider.PublishAuth{
			AccessToken:  token,
			RefreshToken: refreshToken,
		})
		if err != nil {
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil, "")
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
			_ = s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusFailed, "", err.Error(), nil, "")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		if err := s.store.MarkPostTargetResult(ctx, post.ID, account.ID, domain.PostStatusPosted, result.URL, "", result.Metadata, provider.ResolveRemotePostID(account.Provider, result)); err != nil {
			s.logger.Warn("failed to mark post target result", "post_id", post.ID, "account_id", account.ID, "error", err)
			continue
		}
		if strings.TrimSpace(result.URL) != "" {
			utcDay := time.Now().UTC().Format("2006-01-02")
			s.syncOnePostTargetMetrics(ctx, domain.PostedTargetForMetricSync{
				PostID:          post.ID,
				PublishedURL:    result.URL,
				PublishMetadata: result.Metadata,
				Account:         account,
			}, utcDay)
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
		s.postMetricsMu.Lock()
		s.fetchMetricsJob(ctx)
		s.postMetricsMu.Unlock()
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
		s.accountHealthMu.Lock()
		s.accountHealthJob(ctx)
		s.accountHealthMu.Unlock()
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
		s.logger.ErrorContext(ctx, "account metrics list failed", "error", err)
		return
	}
	if len(accounts) == 0 {
		s.logger.DebugContext(ctx, "account metrics sync: no accounts to sync")
		return
	}
	s.logger.InfoContext(ctx, "account metrics sync started", "account_count", len(accounts))
	now := time.Now().UTC()
	synced := 0
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
			s.logger.WarnContext(ctx, "account metrics upsert failed", "account_id", account.ID, "error", err)
			continue
		}
		synced++
	}
	s.logger.InfoContext(ctx, "account metrics sync completed", "synced", synced, "total", len(accounts))
}

// SyncPostMetricsNow pulls engagement metrics for eligible posted targets (admin trigger; bypasses job lock).
func (s *Service) SyncPostMetricsNow(ctx context.Context) {
	s.postMetricsMu.Lock()
	defer s.postMetricsMu.Unlock()
	s.fetchMetricsJob(ctx)
}

// SyncAccountMetricsNow refreshes follower counts for all connected accounts (admin trigger).
func (s *Service) SyncAccountMetricsNow(ctx context.Context) {
	s.accountMetricsMu.Lock()
	defer s.accountMetricsMu.Unlock()
	s.accountMetricsJob(ctx)
}

// fetchMetricsJob pulls provider engagement metrics for posted targets due for refresh.
func (s *Service) fetchMetricsJob(ctx context.Context) {
	since := time.Now().Add(-30 * 24 * time.Hour)
	utcDay := time.Now().UTC().Format("2006-01-02")
	rows, err := s.store.ListPostedTargetsForMetricSync(ctx, since, utcDay, 500)
	if err != nil {
		s.logger.ErrorContext(ctx, "metric sync list failed", "error", err)
		return
	}
	if len(rows) == 0 {
		s.logger.DebugContext(ctx, "post metrics sync: no targets due for refresh")
		return
	}
	s.logger.InfoContext(ctx, "post metrics sync started", "target_count", len(rows))
	for _, row := range rows {
		s.syncOnePostTargetMetrics(ctx, row, utcDay)
	}
	s.logger.InfoContext(ctx, "post metrics sync completed", "target_count", len(rows))
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

func (s *Service) materializeAnnouncement(ctx context.Context, tmpl *domain.PostTemplate, mainEventAt time.Time) error {
	if !tmpl.AnnouncementEnabled || strings.TrimSpace(tmpl.AnnouncementContent) == "" {
		return nil
	}
	annSkipped, err := s.store.IsPostTemplateAnnouncementSkipped(ctx, tmpl.ID, mainEventAt)
	if err != nil {
		return err
	}
	if annSkipped {
		return nil
	}
	exists, err := s.store.HasPostTemplateRoleMaterialized(ctx, tmpl.ID, mainEventAt, domain.TemplatePostRoleAnnouncement)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	daysBefore := tmpl.AnnouncementDaysBefore
	if daysBefore <= 0 {
		daysBefore = 2
	}
	announceAt := mainEventAt.Add(-time.Duration(daysBefore) * 24 * time.Hour)
	counterVal := tmpl.AnnouncementCounterNext
	if counterVal < 1 {
		counterVal = 1
	}
	content := domain.ExpandDynamicVariables(tmpl.AnnouncementContent, announceAt, &counterVal, &mainEventAt)
	expandedTitle := domain.ExpandPostTemplateTitle(tmpl.AnnouncementTitle, announceAt, counterVal, &mainEventAt)
	targets := tmpl.AnnouncementTargetAccountIDs
	if len(targets) == 0 {
		targets = tmpl.TargetAccountIDs
	}

	if s.shouldEnhanceRecurringAnnouncementWithAI(ctx, *tmpl) {
		if err := s.submitRecurringAnnouncementAIEnhancement(ctx, *tmpl, content, expandedTitle, announceAt, mainEventAt); err != nil {
			s.logger.WarnContext(ctx, "recurring materialize: announcement ai unavailable, using template", "template_id", tmpl.ID, "error", err)
		} else {
			return s.store.AdvancePostTemplateAnnouncementCounter(ctx, tmpl.ID, tmpl.AnnouncementCounterNext+1)
		}
	}

	authorID := tmpl.AuthorUserID
	tplID := tmpl.ID
	input := domain.CreatePostInput{
		Title:                 expandedTitle,
		Content:               content,
		ScheduledAt:           announceAt,
		TargetAccounts:        targets,
		Visibility:            tmpl.Visibility,
		MediaIDs:              tmpl.MediaIDs,
		MediaExcludeByAccount: tmpl.MediaExcludeByAccount,
		Draft:                 false,
		AuthorUserID:          &authorID,
		PostTemplateID:        &tplID,
		TemplateCounter:       &counterVal,
		TemplateOccurrenceAt:    &mainEventAt,
		TemplatePostRole:      domain.TemplatePostRoleAnnouncement,
		Source:                domain.PostSourceAutomation,
	}
	principal := domain.AuthenticatedPrincipal{User: domain.User{ID: tmpl.AuthorUserID}}
	if _, err := s.store.CreateScheduledPost(ctx, tmpl.TeamID, principal, input); err != nil {
		return err
	}
	return s.store.AdvancePostTemplateAnnouncementCounter(ctx, tmpl.ID, tmpl.AnnouncementCounterNext+1)
}

func (s *Service) maybeShiftOccurrence(ctx context.Context, tmpl *domain.PostTemplate, occ time.Time) *time.Time {
	shiftTo, err := s.store.GetPostTemplateShiftTo(ctx, tmpl.ID, occ)
	if err != nil {
		s.logger.Warn("template shift lookup failed", "template_id", tmpl.ID, "error", err)
		return nil
	}
	return shiftTo
}

func (s *Service) createScheduledPostFromTemplate(ctx context.Context, tmpl *domain.PostTemplate, scheduledAt time.Time, occurrenceAt time.Time, role string) error {
	counterVal := tmpl.CounterNext
	expandedContent := domain.ExpandDynamicVariables(tmpl.Content, scheduledAt, &counterVal, nil)
	expandedTitle := domain.ExpandPostTemplateTitle(tmpl.Title, scheduledAt, counterVal, nil)
	outputMode := tmpl.OutputMode
	if outputMode == "" {
		outputMode = domain.AutomationOutputScheduled
	} else {
		outputMode = domain.NormalizeAutomationOutputMode(string(outputMode))
	}
	var at time.Time
	var draft bool
	switch outputMode {
	case domain.AutomationOutputDraft:
		at, draft = rssOutputSchedule(outputMode, scheduledAt)
	case domain.AutomationOutputPublishNow:
		at = time.Now().UTC()
		draft = false
	default:
		at = scheduledAt.UTC()
		draft = false
	}

	if s.shouldEnhanceRecurringWithAI(ctx, *tmpl) {
		if err := s.submitRecurringAIEnhancement(ctx, *tmpl, expandedContent, expandedTitle, at, draft, occurrenceAt); err != nil {
			s.logger.WarnContext(ctx, "recurring materialize: ai unavailable, using template", "template_id", tmpl.ID, "error", err)
		} else {
			return nil
		}
	}

	authorID := tmpl.AuthorUserID
	tplID := tmpl.ID
	occAt := occurrenceAt.UTC()
	input := domain.CreatePostInput{
		Title:                 expandedTitle,
		Content:               tmpl.Content,
		ScheduledAt:           at,
		TargetAccounts:        tmpl.TargetAccountIDs,
		Visibility:            tmpl.Visibility,
		MediaIDs:              tmpl.MediaIDs,
		MediaExcludeByAccount: tmpl.MediaExcludeByAccount,
		Draft:                 draft,
		AuthorUserID:          &authorID,
		PostTemplateID:        &tplID,
		TemplateCounter:       &counterVal,
		TemplateOccurrenceAt:  &occAt,
		TemplatePostRole:      role,
		Source:                domain.PostSourceAutomation,
	}
	principal := domain.AuthenticatedPrincipal{User: domain.User{ID: tmpl.AuthorUserID}}
	if _, err := s.store.CreateScheduledPost(ctx, tmpl.TeamID, principal, input); err != nil {
		s.logger.Error("materialize scheduled post from template failed", "template_id", tmpl.ID, "error", err)
		return err
	}
	return nil
}
