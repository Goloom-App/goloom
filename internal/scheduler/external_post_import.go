package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/socialtokens"
)

const externalPostBackfillDays = 30

func (s *Service) runExternalPostImportLoop(ctx context.Context) {
	ticker := time.NewTicker(s.externalPostImportInterval)
	defer ticker.Stop()
	for {
		s.externalPostImportMu.Lock()
		s.externalPostImportJob(ctx)
		s.externalPostImportMu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// SyncExternalPostsNow imports external posts for all enabled teams (admin trigger).
func (s *Service) SyncExternalPostsNow(ctx context.Context) {
	s.externalPostImportMu.Lock()
	defer s.externalPostImportMu.Unlock()
	s.externalPostImportJob(ctx)
}

func (s *Service) externalPostImportJob(ctx context.Context) {
	teams, err := s.store.ListTeamsWithExternalPostMonitorEnabled(ctx, 200)
	if err != nil {
		s.logger.ErrorContext(ctx, "external post import list teams failed", "error", err)
		return
	}
	if len(teams) == 0 {
		s.logger.DebugContext(ctx, "external post import: no teams enabled")
		return
	}
	s.logger.InfoContext(ctx, "external post import started", "team_count", len(teams))
	for _, settings := range teams {
		s.importExternalPostsForTeam(ctx, settings)
	}
	s.logger.InfoContext(ctx, "external post import completed", "team_count", len(teams))
}

func (s *Service) importExternalPostsForTeam(ctx context.Context, settings domain.ExternalPostMonitorSettings) {
	teamID := settings.TeamID
	now := time.Now().UTC()
	since := now.Add(-externalPostBackfillDays * 24 * time.Hour)
	backfillCompleted := settings.BackfillCompletedAt != nil
	if backfillCompleted && settings.LastSyncAt != nil {
		since = settings.LastSyncAt.UTC()
	}

	if removed, err := s.store.DeleteRedundantImportedPosts(ctx, teamID); err != nil {
		s.logger.WarnContext(ctx, "external post import: duplicate cleanup failed", "team_id", teamID, "error", err)
	} else if removed > 0 {
		s.logger.InfoContext(ctx, "external post import: removed duplicate imported posts", "team_id", teamID, "removed", removed)
	}

	ownerID, err := s.teamOwnerUserID(ctx, teamID)
	if err != nil {
		s.logger.WarnContext(ctx, "external post import: no team owner", "team_id", teamID, "error", err)
		return
	}

	accounts, err := s.store.ListTeamAccounts(ctx, teamID)
	if err != nil {
		s.logger.WarnContext(ctx, "external post import: list accounts failed", "team_id", teamID, "error", err)
		return
	}

	imported := 0
	for _, account := range accounts {
		n, err := s.importExternalPostsForAccount(ctx, teamID, ownerID, account, since)
		if err != nil {
			s.logger.WarnContext(ctx, "external post import account failed",
				"team_id", teamID, "account_id", account.ID, "provider", account.Provider, "error", err)
			continue
		}
		imported += n
	}

	if err := s.store.UpdateExternalPostMonitorSyncState(ctx, teamID, now, !backfillCompleted); err != nil {
		s.logger.WarnContext(ctx, "external post import: update sync state failed", "team_id", teamID, "error", err)
	}
	if imported > 0 {
		s.logger.InfoContext(ctx, "external post import team done", "team_id", teamID, "imported", imported)
	}
}

func (s *Service) importExternalPostsForAccount(ctx context.Context, teamID, ownerID string, account domain.SocialAccount, since time.Time) (int, error) {
	providerImpl, ok := s.providers.Get(account.Provider)
	if !ok {
		return 0, fmt.Errorf("unsupported provider %q", account.Provider)
	}
	feedFetcher, ok := providerImpl.(provider.AuthorFeedFetcher)
	if !ok {
		return 0, fmt.Errorf("provider %q does not support author feed", account.Provider)
	}

	acc, err := socialtokens.EnsureMastodonFresh(ctx, s.store, s.providers, account)
	if err != nil {
		return 0, err
	}
	account = acc

	token, err := s.store.DecryptAccessToken(account)
	if err != nil {
		return 0, err
	}
	refreshToken, err := s.store.DecryptRefreshToken(account)
	if err != nil {
		return 0, err
	}

	if strings.TrimSpace(account.RemoteAccountID) == "" {
		return 0, fmt.Errorf("account %s missing remote_account_id", account.ID)
	}

	posts, err := feedFetcher.ListAuthorPosts(ctx, account, provider.PublishAuth{
		AccessToken:  token,
		RefreshToken: refreshToken,
	}, since, 40)
	if err != nil {
		return 0, err
	}

	imported := 0
	for _, ap := range posts {
		exists, err := s.store.AuthorPostAlreadyTracked(ctx, account.ID, ap.RemoteID, ap.URL, ap.Metadata)
		if err != nil {
			s.logger.WarnContext(ctx, "external post import dedup check failed", "remote_id", ap.RemoteID, "error", err)
			continue
		}
		if exists {
			continue
		}
		created, err := s.store.CreateImportedPost(ctx, teamID, ownerID, domain.ImportedPostInput{
			AccountID:       account.ID,
			RemotePostID:    ap.RemoteID,
			Content:         ap.Content,
			PublishedAt:     ap.PublishedAt,
			PublishedURL:    ap.URL,
			PublishMetadata: ap.Metadata,
		})
		if err != nil {
			s.logger.WarnContext(ctx, "external post import create failed", "remote_id", ap.RemoteID, "error", err)
			continue
		}
		imported++
		if strings.TrimSpace(ap.URL) != "" || len(ap.Metadata) > 0 {
			utcDay := created.ScheduledAt.UTC().Format("2006-01-02")
			s.syncOnePostTargetMetrics(ctx, domain.PostedTargetForMetricSync{
				PostID:          created.ID,
				PublishedURL:    ap.URL,
				PublishMetadata: ap.Metadata,
				Account:         account,
			}, utcDay, utcDay)
		}
	}
	return imported, nil
}

func (s *Service) teamOwnerUserID(ctx context.Context, teamID string) (string, error) {
	members, err := s.store.ListTeamMembers(ctx, teamID)
	if err != nil {
		return "", err
	}
	for _, m := range members {
		if m.Role == domain.RoleOwner {
			return m.UserID, nil
		}
	}
	return "", fmt.Errorf("no owner found for team %s", teamID)
}

// ImportOldPostsInput carries parameters for manual old posts import.
type ImportOldPostsInput struct {
	AccountIDs []string `json:"account_ids"`
	Limit      int      `json:"limit"`
	UntilDate  string   `json:"until_date,omitempty"` // optional: "2024-01-01"
}

// ImportOldPostsResult carries the result of a manual old posts import.
type ImportOldPostsResult struct {
	Imported int `json:"imported"`
}

// ImportOldPosts manually imports old posts from specified accounts.
func (s *Service) ImportOldPosts(ctx context.Context, teamID string, input ImportOldPostsInput) (ImportOldPostsResult, error) {
	s.externalPostImportMu.Lock()
	defer s.externalPostImportMu.Unlock()

	ownerID, err := s.teamOwnerUserID(ctx, teamID)
	if err != nil {
		return ImportOldPostsResult{}, fmt.Errorf("no team owner: %w", err)
	}

	accounts, err := s.store.ListTeamAccounts(ctx, teamID)
	if err != nil {
		return ImportOldPostsResult{}, fmt.Errorf("list accounts: %w", err)
	}

	// Filter to requested accounts
	accountMap := make(map[string]struct{}, len(input.AccountIDs))
	for _, id := range input.AccountIDs {
		accountMap[id] = struct{}{}
	}
	var filteredAccounts []domain.SocialAccount
	for _, acc := range accounts {
		if len(input.AccountIDs) == 0 {
			// No filter: import from all accounts
			filteredAccounts = append(filteredAccounts, acc)
		} else if _, ok := accountMap[acc.ID]; ok {
			filteredAccounts = append(filteredAccounts, acc)
		}
	}

	// Parse until date
	var since time.Time
	if input.UntilDate != "" {
		parsed, err := time.Parse("2006-01-02", input.UntilDate)
		if err != nil {
			return ImportOldPostsResult{}, fmt.Errorf("invalid until_date: %w", err)
		}
		since = parsed.UTC()
	} else {
		// Default: 1 year back
		since = time.Now().UTC().AddDate(-1, 0, 0)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	totalImported := 0
	for _, account := range filteredAccounts {
		n, err := s.importOldPostsForAccount(ctx, teamID, ownerID, account, since, limit)
		if err != nil {
			s.logger.WarnContext(ctx, "import old posts account failed",
				"team_id", teamID, "account_id", account.ID, "provider", account.Provider, "error", err)
			continue
		}
		totalImported += n
	}

	if totalImported > 0 {
		s.logger.InfoContext(ctx, "import old posts completed", "team_id", teamID, "imported", totalImported)
	}

	return ImportOldPostsResult{Imported: totalImported}, nil
}

func (s *Service) importOldPostsForAccount(ctx context.Context, teamID, ownerID string, account domain.SocialAccount, since time.Time, limit int) (int, error) {
	providerImpl, ok := s.providers.Get(account.Provider)
	if !ok {
		return 0, fmt.Errorf("unsupported provider %q", account.Provider)
	}
	feedFetcher, ok := providerImpl.(provider.AuthorFeedFetcher)
	if !ok {
		return 0, fmt.Errorf("provider %q does not support author feed", account.Provider)
	}

	acc, err := socialtokens.EnsureMastodonFresh(ctx, s.store, s.providers, account)
	if err != nil {
		return 0, err
	}
	account = acc

	token, err := s.store.DecryptAccessToken(account)
	if err != nil {
		return 0, err
	}
	refreshToken, err := s.store.DecryptRefreshToken(account)
	if err != nil {
		return 0, err
	}

	if strings.TrimSpace(account.RemoteAccountID) == "" {
		return 0, fmt.Errorf("account %s missing remote_account_id", account.ID)
	}

	auth := provider.PublishAuth{
		AccessToken:  token,
		RefreshToken: refreshToken,
	}

	// Paginate: provider returns max ~80-100 posts per request.
	// Use oldest post's PublishedAt as since for next page.
	const pageSize = 80
	totalImported := 0
	currentSince := since

	for totalImported < limit {
		fetchLimit := pageSize
		remaining := limit - totalImported
		if remaining < fetchLimit {
			fetchLimit = remaining
		}

		posts, err := feedFetcher.ListAuthorPosts(ctx, account, auth, currentSince, fetchLimit)
		if err != nil {
			if totalImported > 0 {
				return totalImported, err
			}
			return 0, err
		}
		if len(posts) == 0 {
			break
		}

		batchImported := 0
		var oldestPublishedAt time.Time
		for _, ap := range posts {
			if oldestPublishedAt.IsZero() || ap.PublishedAt.Before(oldestPublishedAt) {
				oldestPublishedAt = ap.PublishedAt
			}

			exists, err := s.store.AuthorPostAlreadyTracked(ctx, account.ID, ap.RemoteID, ap.URL, ap.Metadata)
			if err != nil {
				s.logger.WarnContext(ctx, "import old posts dedup check failed", "remote_id", ap.RemoteID, "error", err)
				continue
			}
			if exists {
				continue
			}
			created, err := s.store.CreateImportedPost(ctx, teamID, ownerID, domain.ImportedPostInput{
				AccountID:       account.ID,
				RemotePostID:    ap.RemoteID,
				Content:         ap.Content,
				PublishedAt:     ap.PublishedAt,
				PublishedURL:    ap.URL,
				PublishMetadata: ap.Metadata,
			})
			if err != nil {
				s.logger.WarnContext(ctx, "import old posts create failed", "remote_id", ap.RemoteID, "error", err)
				continue
			}
			batchImported++
			if strings.TrimSpace(ap.URL) != "" || len(ap.Metadata) > 0 {
				utcDay := created.ScheduledAt.UTC().Format("2006-01-02")
				s.syncOnePostTargetMetrics(ctx, domain.PostedTargetForMetricSync{
					PostID:          created.ID,
					PublishedURL:    ap.URL,
					PublishMetadata: ap.Metadata,
					Account:         account,
				}, utcDay, utcDay)
			}
		}
		totalImported += batchImported

		// If we got fewer posts than requested or no new imports, we've reached the end
		if len(posts) < fetchLimit || batchImported == 0 || oldestPublishedAt.IsZero() {
			break
		}
		// Set since to one second before the oldest post to avoid duplicates
		currentSince = oldestPublishedAt.Add(-1 * time.Second)
	}

	return totalImported, nil
}
