package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) ListOAuthAccountsWithAccessTokenExpiringBefore(ctx context.Context, before time.Time, limit int) ([]domain.AccountOAuthTokenExpiry, error) {
	if limit <= 0 {
		limit = 500
	}
	beforeStr := formatTime(before.UTC())
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, provider, username, access_token_expires_at
		from social_accounts
		where auth_type = 'oauth_token'
		  and access_token_expires_at is not null
		  and trim(access_token_expires_at) != ''
		  and access_token_expires_at < ?
		order by access_token_expires_at asc
		limit ?`,
		beforeStr, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.AccountOAuthTokenExpiry
	for rows.Next() {
		var row domain.AccountOAuthTokenExpiry
		var exp sql.NullString
		if err := rows.Scan(&row.ID, &row.TeamID, &row.Provider, &row.Username, &exp); err != nil {
			return nil, err
		}
		if !exp.Valid || strings.TrimSpace(exp.String) == "" {
			continue
		}
		parsed, err := parseTime(exp.String)
		if err != nil {
			return nil, err
		}
		row.AccessTokenExpiresAt = parsed
		out = append(out, row)
	}
	return out, rows.Err()
}
