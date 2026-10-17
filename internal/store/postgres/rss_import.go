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

// Only unconsumed attempted reservations expire; baseline markers remain permanent. Active jobs
// protect their markers even when generation outlives the fifteen-minute lease.
const rssItemRetryEligible = `(rss_imported_items.consumed_at IS NULL AND (rss_imported_items.retry_available or (rss_imported_items.attempts > 0 and rss_imported_items.created_at <= now() - interval '15 minutes'
 and not exists (select 1 from ai_jobs j join rss_feed_configs f on f.id = rss_imported_items.feed_id
 where j.team_id = f.team_id and j.origin = 'automation' and j.status in ('pending', 'processing')
 and j.payload #>> '{params,rss_automation,feed_id}' = rss_imported_items.feed_id::text
 and j.payload #>> '{params,rss_automation,item_key}' = rss_imported_items.item_key))))`

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
	return s.countRSSFeedBudget(ctx, feedID, time.Now().UTC().Format("2006-01-02"))
}

func (s *Store) countRSSFeedBudget(ctx context.Context, feedID, day string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
 select (select count(*) from rss_imported_items where feed_id = $1 and budget_date = $2::date and not `+rssItemRetryEligible+`)
 + (select count(*) from scheduled_posts p where p.rss_feed_id = $1 and (p.created_at at time zone 'UTC')::date = $2::date
    and not exists (select 1 from rss_imported_items i where i.feed_id = p.rss_feed_id and i.post_id = p.id))`,
		feedID, day).Scan(&count)
	if err != nil {
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
		INSERT INTO rss_imported_items (feed_id, item_key, post_id, consumed_at, budget_date)
		VALUES ($1, $2, $3, CASE WHEN $3::uuid IS NOT NULL THEN now() END, CASE WHEN $3::uuid IS NOT NULL THEN (clock_timestamp() at time zone 'UTC')::date END)
		ON CONFLICT (feed_id, item_key) DO UPDATE SET post_id = excluded.post_id, consumed_at = excluded.consumed_at, retry_available = false, budget_date = coalesce(rss_imported_items.budget_date, excluded.budget_date)
        WHERE rss_imported_items.post_id IS NULL AND rss_imported_items.consumed_at IS NULL AND excluded.post_id IS NOT NULL
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
		SET post_id = $3, consumed_at = coalesce(consumed_at, now()), retry_available = false, budget_date = coalesce(budget_date, (clock_timestamp() at time zone 'UTC')::date)
		WHERE feed_id = $1 AND item_key = $2
		  AND (consumed_at IS NULL OR post_id = $3)
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

// The source lock covers the capacity read and conditional claim across replicas.
func (s *Store) ClaimRSSItemAttempt(ctx context.Context, teamID, feedID, itemKey string) (bool, error) {
	claimed := false
	err := s.WithAuditTransaction(ctx, func(ctx context.Context) error {
		if err := s.LockAutomationSource(ctx, "rss_feed", feedID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		var limit int
		var day string
		err := s.pool.QueryRow(ctx, "select max_posts_per_day, (clock_timestamp() at time zone 'UTC')::date::text from rss_feed_configs where id = $1 and team_id = $2", feedID, teamID).Scan(&limit, &day)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		imported, err := s.RSSItemAlreadyImported(ctx, feedID, itemKey)
		if err != nil || imported {
			return err
		}
		// Use the database UTC day captured after the lock for both statements.
		used, err := s.countRSSFeedBudget(ctx, feedID, day)
		if err != nil {
			return err
		}
		if used >= limit {
			return domain.ErrRSSDailyLimit
		}
		tag, err := s.pool.Exec(ctx, `
 insert into rss_imported_items (feed_id, item_key, attempts, retry_available, budget_date)
 values ($1, $2, 1, false, $3::date)
 on conflict(feed_id, item_key) do update set attempts = rss_imported_items.attempts + 1, retry_available = false, created_at = now(), budget_date = excluded.budget_date
 where rss_imported_items.post_id is null and `+rssItemRetryEligible+` and rss_imported_items.attempts < $4`,
			feedID, itemKey, day, domain.MaxRSSItemAttempts)
		if err != nil {
			return fmt.Errorf("ClaimRSSItemAttempt: %w", err)
		}
		claimed = tag.RowsAffected() > 0
		return nil
	})
	return claimed && err == nil, err
}

func (s *Store) ReleaseRSSItemAttempt(ctx context.Context, teamID, feedID, itemKey string) (int, error) {
	var attempts int
	err := s.pool.QueryRow(ctx, `UPDATE rss_imported_items
 SET retry_available = true, attempts = CASE WHEN attempts = 0 THEN 1 ELSE attempts END
 WHERE feed_id = $1 AND item_key = $2 AND post_id IS NULL AND consumed_at IS NULL
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
	tag, err := tx.Exec(ctx, `INSERT INTO rss_imported_items (feed_id,item_key,post_id,consumed_at,budget_date)
 SELECT $1,$2,$3,now(),(clock_timestamp() at time zone 'UTC')::date WHERE EXISTS (SELECT 1 FROM rss_feed_configs WHERE id=$1 AND team_id=$4)
 ON CONFLICT(feed_id,item_key) DO UPDATE SET post_id=excluded.post_id,consumed_at=excluded.consumed_at,retry_available=false,budget_date=coalesce(rss_imported_items.budget_date,excluded.budget_date)
 WHERE rss_imported_items.post_id IS NULL AND rss_imported_items.consumed_at IS NULL`, feedID, itemKey, postID, teamID)
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
