package postgres

import (
	"context"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) ListOAuthAccountsWithAccessTokenExpiringBefore(ctx context.Context, before time.Time, limit int) ([]domain.AccountOAuthTokenExpiry, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		select id, team_id, provider, username, access_token_expires_at
		from social_accounts
		where auth_type = 'oauth_token'
		  and access_token_expires_at is not null
		  and access_token_expires_at < $1
		order by access_token_expires_at asc
		limit $2`,
		before.UTC(), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.AccountOAuthTokenExpiry
	for rows.Next() {
		var row domain.AccountOAuthTokenExpiry
		var exp *time.Time
		if err := rows.Scan(&row.ID, &row.TeamID, &row.Provider, &row.Username, &exp); err != nil {
			return nil, err
		}
		if exp == nil {
			continue
		}
		row.AccessTokenExpiresAt = exp.UTC()
		out = append(out, row)
	}
	return out, rows.Err()
}
