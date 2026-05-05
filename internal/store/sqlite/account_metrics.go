package sqlite

import (
	"context"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) ListAccountsForMetricsSync(ctx context.Context, limit int) ([]domain.SocialAccount, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts
		order by created_at desc
		limit ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectAccounts(rows)
}

func (s *Store) UpsertAccountMetrics(ctx context.Context, accountID string, metrics map[string]int64, recordedAt time.Time) error {
	if len(metrics) == 0 {
		return nil
	}
	now := nowString()
	day := recordedAt.UTC().Format("2006-01-02")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for name, value := range metrics {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			insert into account_metrics (account_id, metric, value, updated_at)
			values (?, ?, ?, ?)
			on conflict(account_id, metric) do update set
				value = excluded.value,
				updated_at = excluded.updated_at`,
			accountID, name, value, now,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			insert into account_metrics_history (account_id, metric, value, recorded_at)
			values (?, ?, ?, ?)
			on conflict(account_id, metric, recorded_at) do update set
				value = excluded.value`,
			accountID, name, value, day,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetTeamAccountMetricHistorySeries(ctx context.Context, teamID, accountID string, days int) ([]domain.AccountMetricHistoryPoint, error) {
	if days <= 0 {
		days = 30
	}
	if days > 366 {
		days = 366
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	query := `
		select h.recorded_at,
		       sum(case when h.metric = 'followers' then h.value else 0 end) as followers,
		       sum(case when h.metric = 'following' then h.value else 0 end) as following,
		       sum(case when h.metric = 'posts' then h.value else 0 end) as posts
		from account_metrics_history h
		inner join social_accounts a on a.id = h.account_id
		where a.team_id = ? and h.recorded_at >= ?`
	args := []any{teamID, cutoff}
	if strings.TrimSpace(accountID) != "" && strings.TrimSpace(accountID) != "all" {
		query += ` and h.account_id = ?`
		args = append(args, strings.TrimSpace(accountID))
	}
	query += ` group by h.recorded_at order by h.recorded_at asc`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := make([]domain.AccountMetricHistoryPoint, 0)
	for rows.Next() {
		var row domain.AccountMetricHistoryPoint
		if err := rows.Scan(&row.Date, &row.Followers, &row.Following, &row.Posts); err != nil {
			return nil, err
		}
		row.Date = strings.TrimSpace(row.Date)
		points = append(points, row)
	}
	return points, rows.Err()
}
