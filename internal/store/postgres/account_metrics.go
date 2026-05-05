package postgres

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
	rows, err := s.pool.Query(ctx, `
		select id, team_id, provider, auth_type, coalesce(provider_instance_id::text, ''), instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts
		order by created_at desc
		limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]domain.SocialAccount, 0)
	for rows.Next() {
		var account domain.SocialAccount
		var accessExpires *time.Time
		if err := rows.Scan(
			&account.ID,
			&account.TeamID,
			&account.Provider,
			&account.AuthType,
			&account.ProviderInstanceID,
			&account.InstanceURL,
			&account.Username,
			&account.RemoteAccountID,
			&account.AvatarURL,
			&account.AccessTokenCiphertext,
			&account.RefreshTokenCiphertext,
			&account.MaxCharsOverride,
			&accessExpires,
			&account.CreatedAt,
		); err != nil {
			return nil, err
		}
		account.AccessTokenExpiresAt = accessExpires
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (s *Store) UpsertAccountMetrics(ctx context.Context, accountID string, metrics map[string]int64, recordedAt time.Time) error {
	if len(metrics) == 0 {
		return nil
	}
	day := recordedAt.UTC().Format("2006-01-02")
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for name, value := range metrics {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			insert into account_metrics (account_id, metric, value, updated_at)
			values ($1, $2, $3, now())
			on conflict(account_id, metric) do update set
				value = excluded.value,
				updated_at = excluded.updated_at`,
			accountID, name, value,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			insert into account_metrics_history (account_id, metric, value, recorded_at)
			values ($1, $2, $3, $4::date)
			on conflict(account_id, metric, recorded_at) do update set
				value = excluded.value`,
			accountID, name, value, day,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
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
		select h.recorded_at::text,
		       sum(case when h.metric = 'followers' then h.value else 0 end)::bigint as followers,
		       sum(case when h.metric = 'following' then h.value else 0 end)::bigint as following,
		       sum(case when h.metric = 'posts' then h.value else 0 end)::bigint as posts
		from account_metrics_history h
		inner join social_accounts a on a.id = h.account_id
		where a.team_id = $1 and h.recorded_at >= $2::date`
	args := []any{teamID, cutoff}
	if strings.TrimSpace(accountID) != "" && strings.TrimSpace(accountID) != "all" {
		query += ` and h.account_id = $3`
		args = append(args, strings.TrimSpace(accountID))
	}
	query += ` group by h.recorded_at order by h.recorded_at asc`
	rows, err := s.pool.Query(ctx, query, args...)
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
