package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) ListActiveRSSFeedConfigs(ctx context.Context, limit int) ([]domain.RSSFeedConfig, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, rssFeedSelectQuery+`
		where is_active = 1
		order by created_at asc
		limit ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("ListActiveRSSFeedConfigs: %w", err)
	}
	defer rows.Close()
	var feeds []domain.RSSFeedConfig
	for rows.Next() {
		feed, err := scanRSSFeedConfig(rows)
		if err != nil {
			return nil, fmt.Errorf("ListActiveRSSFeedConfigs: scan: %w", err)
		}
		feeds = append(feeds, feed)
	}
	return feeds, rows.Err()
}

func (s *Store) CountRSSFeedPostsToday(ctx context.Context, feedID string) (int, error) {
	start := time.Now().UTC().Truncate(24 * time.Hour)
	var count int
	err := s.db.QueryRowContext(ctx, `
		select count(*)
		from scheduled_posts
		where rss_feed_id = ?
		  and created_at >= ?`,
		feedID, formatTime(start),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountRSSFeedPostsToday: %w", err)
	}
	return count, nil
}

func (s *Store) RSSItemAlreadyImported(ctx context.Context, feedID, itemKey string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `
		select 1 from rss_imported_items where feed_id = ? and item_key = ? limit 1`,
		feedID, itemKey,
	).Scan(&one)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("RSSItemAlreadyImported: %w", err)
	}
	return true, nil
}

func (s *Store) RecordRSSImportedItem(ctx context.Context, feedID, itemKey, postID string) error {
	_, err := s.db.ExecContext(ctx, `
		insert or ignore into rss_imported_items (id, feed_id, item_key, post_id, created_at)
		values (?, ?, ?, ?, ?)`,
		uuid.NewString(), feedID, itemKey, postID, nowString(),
	)
	if err != nil {
		return fmt.Errorf("RecordRSSImportedItem: %w", err)
	}
	return nil
}

func (s *Store) IncrementRSSFeedCounter(ctx context.Context, feedID string) error {
	res, err := s.db.ExecContext(ctx, `
		update rss_feed_configs
		set counter_next = counter_next + 1
		where id = ?`, feedID)
	if err != nil {
		return fmt.Errorf("IncrementRSSFeedCounter: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("IncrementRSSFeedCounter: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("rss feed config not found: %w", sql.ErrNoRows)
	}
	return nil
}

func (s *Store) UpdateRSSFeedLastFetched(ctx context.Context, feedID string, lastFetchedAt time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		update rss_feed_configs
		set last_fetched_at = ?
		where id = ?`, formatTime(lastFetchedAt.UTC()), feedID)
	if err != nil {
		return fmt.Errorf("UpdateRSSFeedLastFetched: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("UpdateRSSFeedLastFetched: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("rss feed config not found: %w", sql.ErrNoRows)
	}
	return nil
}
