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

// rssImportLookback bounds how far an item's pubDate may lie behind the last
// fetch and still count as new. Static-site feeds are often deployed hours
// after the declared pubDate of their newest item, so the pubDate alone cannot
// decide novelty — the rss_imported_items dedupe does. The window only keeps
// the dedupe from re-considering a feed's entire backlog on every poll.
const rssImportLookback = 7 * 24 * time.Hour

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

	since := feed.LastFetchedAt.UTC().Add(-rssImportLookback)
	candidates := filterRSSItemsSince(items, since)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].PublishedAt.Before(candidates[j].PublishedAt)
	})

	processed := 0
	for _, item := range candidates {
		if processed >= remaining {
			break
		}
		created, err := s.createPostFromRSSItem(ctx, feed, item)
		if err != nil {
			s.logger.WarnContext(ctx, "rss import: create post failed", "feed_id", feed.ID, "item_link", item.Link, "error", err)
			continue
		}
		if created {
			processed++
		}
	}

	if err := s.store.UpdateRSSFeedLastFetched(ctx, feed.ID, now); err != nil {
		s.logger.WarnContext(ctx, "rss import: update last fetched failed", "feed_id", feed.ID, "error", err)
	}
}

func (s *Service) handleRSSFirstFetch(ctx context.Context, feed domain.RSSFeedConfig, items []rss.Item, now time.Time, remaining int) {
	publishedKey := ""
	if feed.InitialSyncMode == domain.RSSInitialSyncPublishLatest && len(items) > 0 && remaining > 0 {
		latest := items[0]
		for _, item := range items[1:] {
			if item.PublishedAt.After(latest.PublishedAt) {
				latest = item
			}
		}
		// The published item records itself via createPostFromRSSItem; on
		// failure it stays unrecorded so the next poll can retry it.
		publishedKey = rss.ItemKey(latest)
		if _, err := s.createPostFromRSSItem(ctx, feed, latest); err != nil {
			s.logger.WarnContext(ctx, "rss import: publish latest failed", "feed_id", feed.ID, "error", err)
		}
	}
	// Baseline every current item so the lookback window in later polls does
	// not import the feed's backlog.
	for _, item := range items {
		key := rss.ItemKey(item)
		if key == "" || key == publishedKey {
			continue
		}
		if err := s.store.RecordRSSImportedItem(ctx, feed.ID, key, ""); err != nil {
			s.logger.WarnContext(ctx, "rss import: baseline item failed", "feed_id", feed.ID, "item_key", key, "error", err)
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

// createPostFromRSSItem imports one feed item. It reports whether a post (or
// AI job) was actually created: already-imported items return (false, nil) so
// they do not count against the daily budget.
func (s *Service) createPostFromRSSItem(ctx context.Context, feed domain.RSSFeedConfig, item rss.Item) (bool, error) {
	itemKey := rss.ItemKey(item)
	if itemKey == "" {
		return false, fmt.Errorf("rss item has no guid or link")
	}
	exists, err := s.store.RSSItemAlreadyImported(ctx, feed.ID, itemKey)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	ownerID, err := s.teamOwnerUserID(ctx, feed.TeamID)
	if err != nil {
		return false, err
	}

	fields := rss.ItemFields{
		Title:       item.Title,
		Link:        item.Link,
		Summary:     item.Content,
		FeedName:    feed.Name,
		PublishedAt: item.PublishedAt,
		Counter:     feed.CounterNext,
	}
	content := rss.ExpandContent(feed.NormalizedContentTemplate(), fields)
	content = strings.TrimSpace(content)
	if content == "" {
		return false, fmt.Errorf("rendered rss content is empty")
	}

	scheduledAt, draft := rssOutputSchedule(feed.OutputMode, item.PublishedAt)
	title := strings.TrimSpace(rss.ExpandTitle(feed.NormalizedTitleTemplate(), fields))
	if title == "" {
		title = strings.TrimSpace(item.Title)
	}
	if title == "" {
		title = feed.Name
	}

	if s.shouldEnhanceRSSWithAI(ctx, feed) {
		if err := s.submitRSSAIEnhancement(ctx, feed, item, itemKey, content, title, scheduledAt, draft, ownerID); err != nil {
			s.logger.WarnContext(ctx, "rss import: ai enhancement unavailable, using template", "feed_id", feed.ID, "error", err)
		} else {
			return true, nil
		}
	}

	if err := s.createRSSPostDirect(ctx, feed, itemKey, content, title, scheduledAt, draft, ownerID); err != nil {
		return false, err
	}
	return true, nil
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
