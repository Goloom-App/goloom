package postgres

import (
	"context"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) ListPostedTargetsForMetricSync(ctx context.Context, notBefore time.Time, limit int) ([]domain.PostedTargetForMetricSync, error) {
	if limit <= 0 {
		limit = 500
	}
	const query = `
		select t.post_id, t.published_url,
		       a.id, a.team_id, a.provider, a.auth_type, coalesce(a.provider_instance_id::text, ''), a.instance_url, a.username, a.remote_account_id,
		       a.avatar_url,
		       a.access_token_ciphertext, a.refresh_token_ciphertext, a.max_chars_override, a.created_at
		from scheduled_post_targets t
		inner join scheduled_posts p on p.id = t.post_id
		inner join social_accounts a on a.id = t.account_id
		where t.status = 'posted'
		  and p.status = 'posted'
		  and t.published_url is not null and trim(t.published_url) <> ''
		  and p.updated_at >= $1
		order by p.updated_at desc
		limit $2
	`

	rows, err := s.pool.Query(ctx, query, notBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.PostedTargetForMetricSync
	for rows.Next() {
		var row domain.PostedTargetForMetricSync
		if err := rows.Scan(
			&row.PostID,
			&row.PublishedURL,
			&row.Account.ID,
			&row.Account.TeamID,
			&row.Account.Provider,
			&row.Account.AuthType,
			&row.Account.ProviderInstanceID,
			&row.Account.InstanceURL,
			&row.Account.Username,
			&row.Account.RemoteAccountID,
			&row.Account.AvatarURL,
			&row.Account.AccessTokenCiphertext,
			&row.Account.RefreshTokenCiphertext,
			&row.Account.MaxCharsOverride,
			&row.Account.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UpsertPostMetrics(ctx context.Context, postID, accountID string, metrics map[string]int64) error {
	if len(metrics) == 0 {
		return nil
	}
	for name, val := range metrics {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		_, err := s.pool.Exec(ctx, `
			insert into post_metrics (post_id, account_id, metric, value, updated_at)
			values ($1, $2, $3, $4, now())
			on conflict (post_id, account_id, metric) do update
			set value = excluded.value, updated_at = excluded.updated_at`,
			postID, accountID, name, val,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
