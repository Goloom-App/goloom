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

// Only attempted reservations expire; baseline markers remain permanent. Active jobs
// protect their markers even when generation outlives the fifteen-minute lease.
const rssItemRetryEligible = `(rss_imported_items.retry_available or (rss_imported_items.attempts > 0 and rss_imported_items.created_at <= now() - interval '15 minutes'
 and not exists (select 1 from ai_jobs j join rss_feed_configs f on f.id = rss_imported_items.feed_id
 where j.team_id = f.team_id and j.origin = 'automation' and j.status in ('pending', 'processing')
 and j.payload #>> '{params,rss_automation,feed_id}' = rss_imported_items.feed_id::text
 and j.payload #>> '{params,rss_automation,item_key}' = rss_imported_items.item_key)))`

func (s *Store) ListActiveRSSFeedConfigs(ctx context.Context, limit int) ([]domain.RSSFeedConfig, error) {
	if limit <= 0 {
		limit = 200
	}
	const query = `
		SELECT` + rssFeedConfigSelectList + `
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
	const query = `SELECT 1 FROM rss_imported_items WHERE feed_id = $1 AND item_key = $2 AND (post_id IS NOT NULL OR NOT ` + rssItemRetryEligible + ` OR attempts >= $3) LIMIT 1`
	var one int
	err := s.pool.QueryRow(ctx, query, feedID, itemKey, domain.MaxRSSItemAttempts).Scan(&one)
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
		ON CONFLICT (feed_id, item_key) DO UPDATE SET post_id = excluded.post_id, retry_available = false
        WHERE rss_imported_items.post_id IS NULL AND excluded.post_id IS NOT NULL
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

func (s *Store) UpdateRSSImportedItemPostID(ctx context.Context, teamID, feedID, itemKey, postID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE rss_imported_items
		SET post_id = $3
		WHERE feed_id = $1 AND item_key = $2
		  AND EXISTS (SELECT 1 FROM rss_feed_configs WHERE id = rss_imported_items.feed_id AND team_id = $4)
		  AND EXISTS (SELECT 1 FROM scheduled_posts WHERE id = $3 AND team_id = $4)`, feedID, itemKey, postID, teamID)
	if err != nil {
		return fmt.Errorf("UpdateRSSImportedItemPostID: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rss imported item not found: %w: %w", domain.ErrNotFound, pgx.ErrNoRows)
	}
	return nil
}

func (s *Store) IncrementRSSFeedCounter(ctx context.Context, teamID, feedID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE rss_feed_configs
		SET counter_next = counter_next + 1
		WHERE id = $1 AND team_id = $2`, feedID, teamID)
	if err != nil {
		return fmt.Errorf("IncrementRSSFeedCounter: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rss feed config not found: %w: %w", domain.ErrNotFound, pgx.ErrNoRows)
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

func (s *Store) ClaimRSSItemAttempt(ctx context.Context, teamID, feedID, itemKey string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
 INSERT INTO rss_imported_items (feed_id, item_key, attempts, retry_available)
 SELECT $1, $2, 1, false WHERE EXISTS (SELECT 1 FROM rss_feed_configs WHERE id = $1 AND team_id = $3)
 ON CONFLICT(feed_id, item_key) DO UPDATE SET attempts = rss_imported_items.attempts + 1, retry_available = false, created_at = now()
 WHERE rss_imported_items.post_id IS NULL AND `+rssItemRetryEligible+` AND rss_imported_items.attempts < $4`,
		feedID, itemKey, teamID, domain.MaxRSSItemAttempts)
	if err != nil {
		return false, fmt.Errorf("ClaimRSSItemAttempt: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ReleaseRSSItemAttempt(ctx context.Context, teamID, feedID, itemKey string) (int, error) {
	var attempts int
	err := s.pool.QueryRow(ctx, `UPDATE rss_imported_items
 SET retry_available = true, attempts = CASE WHEN attempts = 0 THEN 1 ELSE attempts END
 WHERE feed_id = $1 AND item_key = $2 AND post_id IS NULL
 AND EXISTS (SELECT 1 FROM rss_feed_configs WHERE id = rss_imported_items.feed_id AND team_id = $3)
 RETURNING attempts`, feedID, itemKey, teamID).Scan(&attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("rss item not found or already linked: %w", domain.ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("ReleaseRSSItemAttempt: %w", err)
	}
	return attempts, nil
}

// Link in the post transaction; concurrent attempts cannot commit a second post.
func linkRSSItemForPost(ctx context.Context, tx pgx.Tx, teamID, feedID, itemKey, postID string) error {
	tag, err := tx.Exec(ctx, `INSERT INTO rss_imported_items (feed_id,item_key,post_id)
 SELECT $1,$2,$3 WHERE EXISTS (SELECT 1 FROM rss_feed_configs WHERE id=$1 AND team_id=$4)
 ON CONFLICT(feed_id,item_key) DO UPDATE SET post_id=excluded.post_id,retry_available=false
 WHERE rss_imported_items.post_id IS NULL`, feedID, itemKey, postID, teamID)
	if err != nil {
		return fmt.Errorf("link RSS item in post transaction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("RSS item already linked or feed not owned: %w", domain.ErrNotFound)
	}
	if _, err := tx.Exec(ctx, `update rss_feed_configs set counter_next = counter_next + 1 where id = $1 and team_id = $2`, feedID, teamID); err != nil {
		return fmt.Errorf("increment RSS counter in post transaction: %w", err)
	}
	return nil
}
