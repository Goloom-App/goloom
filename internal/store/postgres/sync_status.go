package postgres

import (
	"context"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) AdminSyncStatus(ctx context.Context, notBefore time.Time) (domain.AdminSyncStatus, error) {
	now := time.Now().UTC()
	recentPostCutoff := now.Add(-24 * time.Hour)
	recentSyncCutoff := now.Add(-30 * time.Minute)
	olderSyncCutoff := now.Add(-6 * time.Hour)

	var st domain.AdminSyncStatus
	const eligible = `
		t.status = 'posted'
		and p.status = 'posted'
		and t.published_url is not null and trim(t.published_url) <> ''
		and (p.updated_at >= $1 or p.scheduled_at >= $1)
		and (
			t.metrics_last_sync_at is null
			or (p.updated_at >= $2 and t.metrics_last_sync_at <= $3)
			or (p.updated_at < $2 and t.metrics_last_sync_at <= $4)
		)`

	if err := s.pool.QueryRow(ctx, `
		select count(*) from scheduled_post_targets t
		inner join scheduled_posts p on p.id = t.post_id
		where `+eligible,
		notBefore.UTC(), recentPostCutoff, recentSyncCutoff, olderSyncCutoff,
	).Scan(&st.PostedTargetsPendingSync); err != nil {
		return domain.AdminSyncStatus{}, err
	}

	if err := s.pool.QueryRow(ctx, `
		select count(*) from scheduled_post_targets t
		inner join scheduled_posts p on p.id = t.post_id
		where t.status = 'posted' and p.status = 'posted'
		  and t.published_url is not null and trim(t.published_url) <> ''
		  and (p.updated_at >= $1 or p.scheduled_at >= $1)
		  and t.metrics_last_sync_at is null`,
		notBefore.UTC(),
	).Scan(&st.PostedTargetsNeverSynced); err != nil {
		return domain.AdminSyncStatus{}, err
	}

	if err := s.pool.QueryRow(ctx, `
		select count(distinct (t.post_id, t.account_id))
		from scheduled_post_targets t
		inner join scheduled_posts p on p.id = t.post_id
		inner join post_metrics pm on pm.post_id = t.post_id and pm.account_id = t.account_id
		where t.status = 'posted' and p.status = 'posted'`,
	).Scan(&st.PostedTargetsWithMetrics); err != nil {
		return domain.AdminSyncStatus{}, err
	}

	if err := s.pool.QueryRow(ctx, `select count(distinct account_id) from account_metrics`).Scan(&st.AccountsWithFollowerMetrics); err != nil {
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

	rows, err := s.pool.Query(ctx, `
		select account_id, max(updated_at)
		from account_metrics
		where account_id = any($1)
		group by account_id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID string
		var updated time.Time
		if err := rows.Scan(&accountID, &updated); err != nil {
			return err
		}
		if acc, ok := byID[accountID]; ok {
			t := updated.UTC()
			acc.AccountMetricsSyncedAt = &t
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	rows2, err := s.pool.Query(ctx, `
		select account_id, max(metrics_last_sync_at)
		from scheduled_post_targets
		where status = 'posted' and account_id = any($1)
		group by account_id`, ids)
	if err != nil {
		return err
	}
	defer rows2.Close()
	for rows2.Next() {
		var accountID string
		var synced *time.Time
		if err := rows2.Scan(&accountID, &synced); err != nil {
			return err
		}
		if synced == nil {
			continue
		}
		if acc, ok := byID[accountID]; ok {
			t := synced.UTC()
			acc.PostEngagementSyncedAt = &t
		}
	}
	return rows2.Err()
}
