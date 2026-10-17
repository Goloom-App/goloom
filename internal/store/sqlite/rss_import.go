package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

// Only unconsumed attempted reservations expire; baseline markers remain permanent. Active jobs
// protect their markers even when generation outlives the fifteen-minute lease.
const rssItemRetryEligible = `(rss_imported_items.consumed_at is null and (rss_imported_items.retry_available = 1 or (rss_imported_items.attempts > 0 and datetime(rss_imported_items.created_at) <= datetime('now', '-15 minutes')
 and not exists (select 1 from ai_jobs j join rss_feed_configs f on f.id = rss_imported_items.feed_id
 where j.team_id = f.team_id and j.origin = 'automation' and j.status in ('pending', 'processing')
 and json_extract(j.payload, '$.params.rss_automation.feed_id') = rss_imported_items.feed_id
 and json_extract(j.payload, '$.params.rss_automation.item_key') = rss_imported_items.item_key))))`

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
	return s.countRSSFeedBudget(ctx, feedID, time.Now().UTC().Format("2006-01-02"))
}

const rssFeedBudgetQuery = `
 select (select count(*) from rss_imported_items where feed_id = ?1 and budget_date = date(?2) and not ` + rssItemRetryEligible + `)
 + (select count(*) from scheduled_posts p where p.rss_feed_id = ?1 and date(p.created_at) = date(?2)
    and not exists (select 1 from rss_imported_items i where i.feed_id = p.rss_feed_id and i.post_id = p.id))`

func (s *Store) countRSSFeedBudget(ctx context.Context, feedID, day string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, rssFeedBudgetQuery, feedID, day).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountRSSFeedPostsToday: %w", err)
	}
	return count, nil
}

func (s *Store) RSSItemAlreadyImported(ctx context.Context, feedID, itemKey string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `
		select 1 from rss_imported_items where feed_id = ? and item_key = ? and (post_id is not null or not `+rssItemRetryEligible+` or attempts >= ?) limit 1`,
		feedID, itemKey, domain.MaxRSSItemAttempts,
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
	var postArg, consumedAt any
	if strings.TrimSpace(postID) != "" {
		postArg = postID
		consumedAt = nowString()
	}
	_, err := s.db.ExecContext(ctx, `
		insert into rss_imported_items (id, feed_id, item_key, post_id, consumed_at, created_at, budget_date)
		values (?, ?, ?, ?, ?, ?, case when ? is not null then date('now') end)
        on conflict(feed_id, item_key) do update set post_id = excluded.post_id, consumed_at = excluded.consumed_at, retry_available = 0, budget_date = coalesce(rss_imported_items.budget_date, excluded.budget_date)
        where rss_imported_items.post_id is null and rss_imported_items.consumed_at is null and excluded.post_id is not null`,
		uuid.NewString(), feedID, itemKey, postArg, consumedAt, nowString(), postArg,
	)
	if err != nil {
		return fmt.Errorf("RecordRSSImportedItem: %w", err)
	}
	return nil
}

func (s *Store) UpdateRSSImportedItemPostID(ctx context.Context, teamID, feedID, itemKey, postID string) error {
	res, err := s.db.ExecContext(ctx, `
		update rss_imported_items
		set post_id = ?, consumed_at = coalesce(consumed_at, datetime('now')), retry_available = 0, budget_date = coalesce(budget_date, date('now'))
		where feed_id = ? and item_key = ?
		  and (consumed_at is null or post_id = ?)
		  and exists (select 1 from rss_feed_configs where id = rss_imported_items.feed_id and team_id = ?)
		  and exists (select 1 from scheduled_posts where id = ? and team_id = ?)`,
		postID, feedID, itemKey, postID, teamID, postID, teamID,
	)
	if err != nil {
		return fmt.Errorf("UpdateRSSImportedItemPostID: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("UpdateRSSImportedItemPostID: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("rss imported item not found: %w: %w", domain.ErrNotFound, sql.ErrNoRows)
	}
	return nil
}

func (s *Store) IncrementRSSFeedCounter(ctx context.Context, teamID, feedID string) error {
	res, err := s.db.ExecContext(ctx, `
		update rss_feed_configs
		set counter_next = counter_next + 1
		where id = ? and team_id = ?`, feedID, teamID)
	if err != nil {
		return fmt.Errorf("IncrementRSSFeedCounter: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("IncrementRSSFeedCounter: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("rss feed config not found: %w: %w", domain.ErrNotFound, sql.ErrNoRows)
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

// A single write statement checks capacity and claims the item under SQLite's
// writer lock, including callers outside an existing source transaction.
func (s *Store) ClaimRSSItemAttempt(ctx context.Context, teamID, feedID, itemKey string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
 insert into rss_imported_items (id, feed_id, item_key, attempts, retry_available, created_at, budget_date)
 select ?5, ?1, ?4, 1, 0, ?6, date(?2) from rss_feed_configs
 where id = ?1 and team_id = ?3 and max_posts_per_day > (`+rssFeedBudgetQuery+`)
 on conflict(feed_id, item_key) do update set attempts = attempts + 1, retry_available = 0, created_at = excluded.created_at, budget_date = excluded.budget_date
 where rss_imported_items.post_id is null and `+rssItemRetryEligible+` and rss_imported_items.attempts < ?7`,
		feedID, "now", teamID, itemKey, uuid.NewString(), nowString(), domain.MaxRSSItemAttempts)
	if err != nil {
		return false, fmt.Errorf("ClaimRSSItemAttempt: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil || n > 0 {
		return n > 0, err
	}
	// Distinguish a full budget from dedupe/ownership rejection for observability.
	var owned bool
	if err := s.db.QueryRowContext(ctx, "select exists(select 1 from rss_feed_configs where id = ? and team_id = ?)", feedID, teamID).Scan(&owned); err != nil {
		return false, err
	}
	if !owned {
		return false, nil
	}
	imported, err := s.RSSItemAlreadyImported(ctx, feedID, itemKey)
	if err != nil || imported {
		return false, err
	}
	return false, domain.ErrRSSDailyLimit
}

func (s *Store) ReleaseRSSItemAttempt(ctx context.Context, teamID, feedID, itemKey string) (int, error) {
	var attempts int
	err := s.db.QueryRowContext(ctx, `update rss_imported_items
 set retry_available = 1, attempts = case when attempts = 0 then 1 else attempts end
 where feed_id = ? and item_key = ? and post_id is null and consumed_at is null
 and exists (select 1 from rss_feed_configs where id = rss_imported_items.feed_id and team_id = ?)
 returning attempts`, feedID, itemKey, teamID).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("rss item not found or already linked: %w", domain.ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("ReleaseRSSItemAttempt: %w", err)
	}
	return attempts, nil
}

// Link in the post transaction so a committed post can never be released for retry,
// including when the post's response read fails after commit.
func linkRSSItemForPost(ctx context.Context, tx sqlExecutor, teamID, feedID, itemKey, postID string) error {
	res, err := tx.ExecContext(ctx, `insert into rss_imported_items (id, feed_id, item_key, post_id, consumed_at, created_at, budget_date)
 select ?, ?, ?, ?, ?, ?, date('now') where exists (select 1 from rss_feed_configs where id = ? and team_id = ?)
 on conflict(feed_id, item_key) do update set post_id = excluded.post_id, consumed_at = excluded.consumed_at, retry_available = 0, budget_date = coalesce(rss_imported_items.budget_date, excluded.budget_date)
 where rss_imported_items.post_id is null and rss_imported_items.consumed_at is null`, uuid.NewString(), feedID, itemKey, postID, nowString(), nowString(), feedID, teamID)
	if err != nil {
		return fmt.Errorf("link RSS item in post transaction: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("RSS item already linked or feed not owned: %w", domain.ErrNotFound)
	}
	if _, err := tx.ExecContext(ctx, `update rss_feed_configs set counter_next = counter_next + 1 where id = ? and team_id = ?`, feedID, teamID); err != nil {
		return fmt.Errorf("increment RSS counter in post transaction: %w", err)
	}
	return nil
}
