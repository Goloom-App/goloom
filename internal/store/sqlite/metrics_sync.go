package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) ListPostedTargetsForMetricSync(ctx context.Context, notBefore time.Time, utcDay string, limit int) ([]domain.PostedTargetForMetricSync, error) {
	if limit <= 0 {
		limit = 500
	}
	since := formatTime(notBefore.UTC())
	now := time.Now().UTC()
	recentPostCutoff := formatTime(now.Add(-24 * time.Hour))
	recentSyncCutoff := formatTime(now.Add(-30 * time.Minute))
	olderSyncCutoff := formatTime(now.Add(-6 * time.Hour))
	utcDay = strings.TrimSpace(utcDay)
	if utcDay == "" {
		utcDay = time.Now().UTC().Format("2006-01-02")
	}
	_ = utcDay
	rows, err := s.db.QueryContext(ctx, `
		select t.post_id, t.published_url,
		       a.id, a.team_id, a.provider, a.auth_type, a.provider_instance_id, a.instance_url, a.username, a.remote_account_id,
		       a.avatar_url,
		       a.access_token_ciphertext, a.refresh_token_ciphertext, a.max_chars_override, a.created_at
		from scheduled_post_targets t
		inner join scheduled_posts p on p.id = t.post_id
		inner join social_accounts a on a.id = t.account_id
		where t.status = 'posted'
		  and p.status = 'posted'
		  and t.published_url is not null and trim(t.published_url) <> ''
		  and p.updated_at >= ?
		  and (
			t.metrics_last_sync_at is null
			or (
				p.updated_at >= ?
				and t.metrics_last_sync_at <= ?
			)
			or (
				p.updated_at < ?
				and t.metrics_last_sync_at <= ?
			)
		  )
		order by p.updated_at desc
		limit ?`,
		since, recentPostCutoff, recentSyncCutoff, recentPostCutoff, olderSyncCutoff, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.PostedTargetForMetricSync
	for rows.Next() {
		var row domain.PostedTargetForMetricSync
		var providerInstanceID sql.NullString
		var maxChars sql.NullInt64
		var createdAt string
		if err := rows.Scan(
			&row.PostID,
			&row.PublishedURL,
			&row.Account.ID,
			&row.Account.TeamID,
			&row.Account.Provider,
			&row.Account.AuthType,
			&providerInstanceID,
			&row.Account.InstanceURL,
			&row.Account.Username,
			&row.Account.RemoteAccountID,
			&row.Account.AvatarURL,
			&row.Account.AccessTokenCiphertext,
			&row.Account.RefreshTokenCiphertext,
			&maxChars,
			&createdAt,
		); err != nil {
			return nil, err
		}
		row.Account.ProviderInstanceID = providerInstanceID.String
		if maxChars.Valid {
			v := int(maxChars.Int64)
			row.Account.MaxCharsOverride = &v
		}
		parsed, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		row.Account.CreatedAt = parsed
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UpsertPostMetrics(ctx context.Context, postID, accountID string, metrics map[string]int64) error {
	if len(metrics) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := nowString()
	utcDay := time.Now().UTC().Format("2006-01-02")
	for name, val := range metrics {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		_, err := tx.ExecContext(ctx, `
			insert into post_metrics (post_id, account_id, metric, value, updated_at)
			values (?, ?, ?, ?, ?)
			on conflict(post_id, account_id, metric) do update set
				value = excluded.value,
				updated_at = excluded.updated_at`,
			postID, accountID, name, val, now,
		)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			insert into post_metrics_history (post_id, account_id, metric, value, recorded_at)
			values (?, ?, ?, ?, ?)
			on conflict(post_id, account_id, metric, recorded_at) do update set
				value = excluded.value`,
			postID, accountID, name, val, utcDay,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) MarkScheduledPostTargetMetricsSynced(ctx context.Context, postID, accountID, utcDay string) error {
	utcDay = strings.TrimSpace(utcDay)
	if utcDay == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		update scheduled_post_targets
		set metrics_last_sync_date = ?, metrics_last_sync_at = ?
		where post_id = ? and account_id = ?`,
		utcDay, nowString(), postID, accountID,
	)
	return err
}
