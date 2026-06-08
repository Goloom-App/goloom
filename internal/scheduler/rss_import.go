package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/rss"
)

type rssFeedFetcher interface {
	Fetch(ctx context.Context, feedURL string) ([]rss.Item, error)
}

func (s *Service) runRSSImportLoop(ctx context.Context) {
	s.rssImportJob(ctx)

	ticker := time.NewTicker(s.rssImportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.rssImportMu.Lock()
			s.rssImportJob(ctx)
			s.rssImportMu.Unlock()
		}
	}
}

// SyncRSSFeedsNow polls all active RSS feeds immediately (admin trigger).
func (s *Service) SyncRSSFeedsNow(ctx context.Context) {
	s.rssImportMu.Lock()
	defer s.rssImportMu.Unlock()
	s.rssImportJob(ctx)
}

func (s *Service) rssImportJob(ctx context.Context) {
	parser := rss.NewParser()
	feeds, err := s.store.ListActiveRSSFeedConfigs(ctx, 200)
	if err != nil {
		s.logger.ErrorContext(ctx, "rss import list feeds failed", "error", err)
		return
	}
	if len(feeds) == 0 {
		return
	}
	s.logger.InfoContext(ctx, "rss import started", "feed_count", len(feeds))
	for _, feed := range feeds {
		s.importRSSFeed(ctx, parser, feed)
	}
	s.logger.InfoContext(ctx, "rss import completed", "feed_count", len(feeds))
}

func (s *Service) importRSSFeed(ctx context.Context, parser rssFeedFetcher, feed domain.RSSFeedConfig) {
	if len(feed.TargetAccountIDs) == 0 {
		s.logger.WarnContext(ctx, "rss import: feed has no target accounts", "feed_id", feed.ID, "team_id", feed.TeamID)
		return
	}

	now := time.Now().UTC()
	createdToday, err := s.store.CountRSSFeedPostsToday(ctx, feed.ID)
	if err != nil {
		s.logger.WarnContext(ctx, "rss import: count posts failed", "feed_id", feed.ID, "error", err)
		return
	}
	remaining := feed.NormalizedMaxPostsPerDay() - createdToday
	if remaining <= 0 {
		s.logger.DebugContext(ctx, "rss import: daily limit reached", "feed_id", feed.ID)
		return
	}

	items, err := parser.Fetch(ctx, feed.FeedURL)
	if err != nil {
		s.logger.WarnContext(ctx, "rss import: fetch failed", "feed_id", feed.ID, "feed_url", feed.FeedURL, "error", err)
		return
	}

	if feed.LastFetchedAt == nil {
		s.handleRSSFirstFetch(ctx, feed, items, now, remaining)
		return
	}

	since := feed.LastFetchedAt.UTC()
	candidates := filterRSSItemsSince(items, since)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].PublishedAt.Before(candidates[j].PublishedAt)
	})

	processed := 0
	for _, item := range candidates {
		if processed >= remaining {
			break
		}
		if err := s.createPostFromRSSItem(ctx, feed, item); err != nil {
			s.logger.WarnContext(ctx, "rss import: create post failed", "feed_id", feed.ID, "item_link", item.Link, "error", err)
			continue
		}
		processed++
	}

	if err := s.store.UpdateRSSFeedLastFetched(ctx, feed.ID, now); err != nil {
		s.logger.WarnContext(ctx, "rss import: update last fetched failed", "feed_id", feed.ID, "error", err)
	}
}

func (s *Service) handleRSSFirstFetch(ctx context.Context, feed domain.RSSFeedConfig, items []rss.Item, now time.Time, remaining int) {
	if feed.InitialSyncMode == domain.RSSInitialSyncPublishLatest && len(items) > 0 && remaining > 0 {
		latest := items[0]
		for _, item := range items[1:] {
			if item.PublishedAt.After(latest.PublishedAt) {
				latest = item
			}
		}
		if err := s.createPostFromRSSItem(ctx, feed, latest); err != nil {
			s.logger.WarnContext(ctx, "rss import: publish latest failed", "feed_id", feed.ID, "error", err)
		}
	}
	if err := s.store.UpdateRSSFeedLastFetched(ctx, feed.ID, now); err != nil {
		s.logger.WarnContext(ctx, "rss import: baseline feed failed", "feed_id", feed.ID, "error", err)
	}
}

func filterRSSItemsSince(items []rss.Item, since time.Time) []rss.Item {
	out := make([]rss.Item, 0)
	for _, item := range items {
		if item.PublishedAt.UTC().After(since) {
			out = append(out, item)
		}
	}
	return out
}

func (s *Service) createPostFromRSSItem(ctx context.Context, feed domain.RSSFeedConfig, item rss.Item) error {
	itemKey := rss.ItemKey(item)
	if itemKey == "" {
		return fmt.Errorf("rss item has no guid or link")
	}
	exists, err := s.store.RSSItemAlreadyImported(ctx, feed.ID, itemKey)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	ownerID, err := s.teamOwnerUserID(ctx, feed.TeamID)
	if err != nil {
		return err
	}

	content := rss.ExpandContent(feed.NormalizedContentTemplate(), rss.ItemFields{
		Title:       item.Title,
		Link:        item.Link,
		Summary:     item.Content,
		FeedName:    feed.Name,
		PublishedAt: item.PublishedAt,
		Counter:     feed.CounterNext,
	})
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("rendered rss content is empty")
	}

	scheduledAt, draft := rssOutputSchedule(feed.OutputMode, item.PublishedAt)
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = feed.Name
	}

	feedID := feed.ID
	input := domain.CreatePostInput{
		Title:          title,
		Content:        content,
		ScheduledAt:    scheduledAt,
		TargetAccounts: feed.TargetAccountIDs,
		Draft:          draft,
		AuthorUserID:   &ownerID,
		Source:         domain.PostSourceAutomation,
		RSSFeedID:      &feedID,
	}
	principal := domain.AuthenticatedPrincipal{User: domain.User{ID: ownerID}, Kind: "system"}
	post, err := s.store.CreateScheduledPost(ctx, feed.TeamID, principal, input)
	if err != nil {
		return err
	}
	if err := s.store.RecordRSSImportedItem(ctx, feed.ID, itemKey, post.ID); err != nil {
		return err
	}
	if err := s.store.IncrementRSSFeedCounter(ctx, feed.ID); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "rss import: post created", "feed_id", feed.ID, "post_id", post.ID, "status", post.Status)
	return nil
}

func rssOutputSchedule(mode domain.AutomationOutputMode, publishedAt time.Time) (time.Time, bool) {
	now := time.Now().UTC()
	switch mode {
	case domain.AutomationOutputDraft:
		at := publishedAt.UTC()
		if at.After(now) {
			return at, true
		}
		return now, true
	case domain.AutomationOutputScheduled:
		at := publishedAt.UTC()
		if at.After(now) {
			return at, false
		}
		return now, false
	default:
		return now, false
	}
}
