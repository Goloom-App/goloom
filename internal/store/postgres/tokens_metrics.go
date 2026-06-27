package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"github.com/google/uuid"
)

func randomAPIToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "gl_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Store) AdminMetrics(ctx context.Context) (domain.AdminMetrics, error) {
	var m domain.AdminMetrics
	if err := s.pool.QueryRow(ctx, `select count(*) from users`).Scan(&m.UsersCount); err != nil {
		return domain.AdminMetrics{}, err
	}
	if err := s.pool.QueryRow(ctx, `select count(*) from teams`).Scan(&m.TeamsCount); err != nil {
		return domain.AdminMetrics{}, err
	}
	if err := s.pool.QueryRow(ctx, `select count(*) from provider_instances`).Scan(&m.ProviderInstancesCount); err != nil {
		return domain.AdminMetrics{}, err
	}

	rows, err := s.pool.Query(ctx, `select status, count(*) from scheduled_posts group by status`)
	if err != nil {
		return domain.AdminMetrics{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int64
		if err := rows.Scan(&status, &n); err != nil {
			return domain.AdminMetrics{}, err
		}
		switch domain.PostStatus(status) {
		case domain.PostStatusPending:
			m.PostsPending = n
		case domain.PostStatusDraft:
			m.PostsDraft = n
		case domain.PostStatusProcessing:
			m.PostsProcessing = n
		case domain.PostStatusPosted:
			m.PostsPosted = n
		case domain.PostStatusFailed:
			m.PostsFailed = n
		case domain.PostStatusCancelled:
			m.PostsCancelled = n
		}
	}
	if err := rows.Err(); err != nil {
		return domain.AdminMetrics{}, err
	}
	// Acknowledged failures no longer need attention, so exclude them from the
	// failed count that drives the admin health banner.
	if err := s.pool.QueryRow(ctx,
		`select count(*) from scheduled_posts where status = 'failed' and acknowledged_at is null`,
	).Scan(&m.PostsFailed); err != nil {
		return domain.AdminMetrics{}, err
	}
	return m, nil
}

func (s *Store) RepairFuturePostedPosts(ctx context.Context) (int64, error) {
	res, err := s.pool.Exec(ctx, `
		update scheduled_posts
		set status = $1
		where status = $2
		  and scheduled_at > now()`,
		domain.PostStatusPending, domain.PostStatusPosted,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func (s *Store) CreateUserAPIToken(ctx context.Context, userID, name string, expiresAt *time.Time, scopes string, teamID *string, description string) (string, domain.APIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", domain.APIToken{}, errors.New("name is required")
	}
	description = strings.TrimSpace(description)
	exp := expiresAt
	if exp == nil {
		t := time.Now().UTC().AddDate(0, 0, 90)
		exp = &t
	}
	plaintext, err := randomAPIToken()
	if err != nil {
		return "", domain.APIToken{}, err
	}
	hash := security.HashToken(plaintext)
	id := uuid.NewString()
	var createdAt time.Time
	var storedExpires time.Time
	err = s.pool.QueryRow(ctx, `
		insert into api_tokens (id, user_id, name, token_hash, expires_at, scopes, description, team_id, created_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, now())
		returning created_at, expires_at`,
		id, userID, name, hash, *exp, scopes, description, teamID,
	).Scan(&createdAt, &storedExpires)
	if err != nil {
		return "", domain.APIToken{}, err
	}
	parsedScopes, _ := parseTokenScopes(scopes)
	return plaintext, domain.APIToken{
		ID:          id,
		UserID:      userID,
		Name:        name,
		Description: description,
		TeamID:      teamID,
		Scopes:      parsedScopes,
		ExpiresAt:   &storedExpires,
		CreatedAt:   createdAt,
	}, nil
}

func (s *Store) TryAcquireLock(ctx context.Context, lockID string, duration time.Duration) (bool, error) {
	const query = `
		insert into job_locks (lock_id, expires_at)
		values ($1, now() + $2)
		on conflict (lock_id) do update
		set locked_at = now(), expires_at = now() + $2
		where job_locks.expires_at < now()
		returning lock_id
	`
	var returnedID string
	err := s.pool.QueryRow(ctx, query, lockID, duration).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return returnedID == lockID, nil
}

func (s *Store) CreateSessionAPIToken(ctx context.Context, userID string, ttl time.Duration) (string, domain.APIToken, error) {
	if ttl <= 0 {
		ttl = s.webSessionTTL()
	}
	expires := time.Now().UTC().Add(ttl)
	return s.CreateUserAPIToken(ctx, userID, domain.WebSessionAPITokenName, &expires, "", nil, "")
}

func (s *Store) ListUserAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	if _, err := s.pool.Exec(ctx, `
		delete from api_tokens
		where user_id = $1
		  and name = $2
		  and expires_at is not null
		  and expires_at <= now()`,
		userID, domain.WebSessionAPITokenName,
	); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		select id, user_id, name, description, scopes, team_id, last_used_at, expires_at, created_at
		from api_tokens
		where user_id = $1
		order by created_at desc`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.APIToken
	for rows.Next() {
		var t domain.APIToken
		var teamID *string
		var description, scopes string
		var lastUsed, expires *time.Time
		var created time.Time
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &description, &scopes, &teamID, &lastUsed, &expires, &created); err != nil {
			return nil, err
		}
		t.Description = description
		if parsed, perr := parseTokenScopes(scopes); perr == nil {
			t.Scopes = parsed
		}
		t.TeamID = teamID
		t.LastUsedAt = lastUsed
		t.ExpiresAt = expires
		t.CreatedAt = created
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeUserAPIToken(ctx context.Context, userID, tokenID string) error {
	tag, err := s.pool.Exec(ctx, `delete from api_tokens where id = $1 and user_id = $2`, tokenID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}
