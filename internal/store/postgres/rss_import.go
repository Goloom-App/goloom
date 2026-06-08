package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListActiveRSSFeedConfigs(ctx context.Context, limit int) ([]domain.RSSFeedConfig, error) {
	if limit <= 0 {
		limit = 200
	}
	const query = `
		SELECT id, team_id, feed_url, name, is_active, content_template, output_mode, max_posts_per_day, counter_next,
		       prompt_hint, target_account_ids, tonality, initial_sync_mode, last_fetched_at, created_at
		FROM rss_feed_configs
		WHERE is_active = true
		ORDER BY created_at ASC
		LIMIT $1
	`
	rows, err := s.pool.Query(ctx, query, limit)
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
	const query = `
		SELECT count(*)
		FROM scheduled_posts
		WHERE rss_feed_id = $1
		  AND created_at >= date_trunc('day', now() at time zone 'utc')
	`
	var count int
	if err := s.pool.QueryRow(ctx, query, feedID).Scan(&count); err != nil {
		return 0, fmt.Errorf("CountRSSFeedPostsToday: %w", err)
	}
	return count, nil
}

func (s *Store) RSSItemAlreadyImported(ctx context.Context, feedID, itemKey string) (bool, error) {
	const query = `SELECT 1 FROM rss_imported_items WHERE feed_id = $1 AND item_key = $2 LIMIT 1`
	var one int
	err := s.pool.QueryRow(ctx, query, feedID, itemKey).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("RSSItemAlreadyImported: %w", err)
	}
	return true, nil
}

func (s *Store) RecordRSSImportedItem(ctx context.Context, feedID, itemKey, postID string) error {
	const query = `
		INSERT INTO rss_imported_items (feed_id, item_key, post_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (feed_id, item_key) DO NOTHING
	`
	var postArg any
	if strings.TrimSpace(postID) != "" {
		postArg = postID
	}
	if _, err := s.pool.Exec(ctx, query, feedID, itemKey, postArg); err != nil {
		return fmt.Errorf("RecordRSSImportedItem: %w", err)
	}
	return nil
}

func (s *Store) UpdateRSSImportedItemPostID(ctx context.Context, feedID, itemKey, postID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE rss_imported_items
		SET post_id = $3
		WHERE feed_id = $1 AND item_key = $2`, feedID, itemKey, postID)
	if err != nil {
		return fmt.Errorf("UpdateRSSImportedItemPostID: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rss imported item not found: %w", pgx.ErrNoRows)
	}
	return nil
}

func (s *Store) IncrementRSSFeedCounter(ctx context.Context, feedID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE rss_feed_configs
		SET counter_next = counter_next + 1
		WHERE id = $1`, feedID)
	if err != nil {
		return fmt.Errorf("IncrementRSSFeedCounter: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rss feed config not found: %w", pgx.ErrNoRows)
	}
	return nil
}

func (s *Store) UpdateRSSFeedLastFetched(ctx context.Context, feedID string, lastFetchedAt time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE rss_feed_configs
		SET last_fetched_at = $2
		WHERE id = $1`, feedID, lastFetchedAt.UTC())
	if err != nil {
		return fmt.Errorf("UpdateRSSFeedLastFetched: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rss feed config not found: %w", pgx.ErrNoRows)
	}
	return nil
}
