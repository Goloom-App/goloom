package sqlite

import (
	"context"
	"database/sql"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) AdminSyncStatus(ctx context.Context, notBefore time.Time) (domain.AdminSyncStatus, error) {
	since := formatTime(notBefore.UTC())
	now := time.Now().UTC()
	recentPostCutoff := formatTime(now.Add(-72 * time.Hour))
	recentSyncCutoff := formatTime(now.Add(-10 * time.Minute))
	olderSyncCutoff := formatTime(now.Add(-2 * time.Hour))
	publishedAtExpr := `case when p.updated_at >= p.scheduled_at then p.updated_at else p.scheduled_at end`

	var st domain.AdminSyncStatus
	eligible := `
		t.status = 'posted'
		and p.status = 'posted'
		and t.published_url is not null and trim(t.published_url) <> ''
		and (p.updated_at >= ? or p.scheduled_at >= ?)
		and (
			t.metrics_last_sync_at is null
			or (` + publishedAtExpr + ` >= ? and t.metrics_last_sync_at <= ?)
			or (` + publishedAtExpr + ` < ? and t.metrics_last_sync_at <= ?)
		)`

	if err := s.db.QueryRowContext(ctx, `
		select count(*) from scheduled_post_targets t
		inner join scheduled_posts p on p.id = t.post_id
		where `+eligible,
		since, since, recentPostCutoff, recentSyncCutoff, recentPostCutoff, olderSyncCutoff,
	).Scan(&st.PostedTargetsPendingSync); err != nil {
		return domain.AdminSyncStatus{}, err
	}

	if err := s.db.QueryRowContext(ctx, `
		select count(*) from scheduled_post_targets t
		inner join scheduled_posts p on p.id = t.post_id
		where t.status = 'posted' and p.status = 'posted'
		  and t.published_url is not null and trim(t.published_url) <> ''
		  and (p.updated_at >= ? or p.scheduled_at >= ?)
		  and t.metrics_last_sync_at is null`,
		since, since,
	).Scan(&st.PostedTargetsNeverSynced); err != nil {
		return domain.AdminSyncStatus{}, err
	}

	if err := s.db.QueryRowContext(ctx, `
		select count(distinct t.post_id || ':' || t.account_id)
		from scheduled_post_targets t
		inner join scheduled_posts p on p.id = t.post_id
		inner join post_metrics pm on pm.post_id = t.post_id and pm.account_id = t.account_id
		where t.status = 'posted' and p.status = 'posted'`,
	).Scan(&st.PostedTargetsWithMetrics); err != nil {
		return domain.AdminSyncStatus{}, err
	}

	if err := s.db.QueryRowContext(ctx, `select count(distinct account_id) from account_metrics`).Scan(&st.AccountsWithFollowerMetrics); err != nil {
		return domain.AdminSyncStatus{}, err
	}

	return st, nil
}

func (s *Store) FillAccountSyncTimestamps(ctx context.Context, accounts []domain.SocialAccount) error {
	if len(accounts) == 0 {
		return nil
	}
	ids := make([]string, len(accounts))
	byID := make(map[string]*domain.SocialAccount, len(accounts))
	for i := range accounts {
		ids[i] = accounts[i].ID
		byID[accounts[i].ID] = &accounts[i]
	}

	placeholders, args := inClause(ids)
	rows, err := s.db.QueryContext(ctx, `
		select account_id, max(updated_at)
		from account_metrics
		where account_id in (`+placeholders+`)
		group by account_id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID string
		var updated sql.NullString
		if err := rows.Scan(&accountID, &updated); err != nil {
			return err
		}
		acc, ok := byID[accountID]
		if !ok || !updated.Valid || updated.String == "" {
			continue
		}
		parsed, err := parseTime(updated.String)
		if err != nil {
			return err
		}
		acc.AccountMetricsSyncedAt = &parsed
	}
	if err := rows.Err(); err != nil {
		return err
	}

	rows2, err := s.db.QueryContext(ctx, `
		select account_id, max(metrics_last_sync_at)
		from scheduled_post_targets
		where status = 'posted' and account_id in (`+placeholders+`)
		group by account_id`, args...)
	if err != nil {
		return err
	}
	defer rows2.Close()
	for rows2.Next() {
		var accountID string
		var synced sql.NullString
		if err := rows2.Scan(&accountID, &synced); err != nil {
			return err
		}
		acc, ok := byID[accountID]
		if !ok || !synced.Valid || synced.String == "" {
			continue
		}
		parsed, err := parseTime(synced.String)
		if err != nil {
			return err
		}
		acc.PostEngagementSyncedAt = &parsed
	}
	return rows2.Err()
}
