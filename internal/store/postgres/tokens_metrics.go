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
	return m, rows.Err()
}

func (s *Store) CreateUserAPIToken(ctx context.Context, userID, name string, expiresAt *time.Time) (string, domain.APIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", domain.APIToken{}, errors.New("name is required")
	}
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
		insert into api_tokens (id, user_id, name, token_hash, expires_at, created_at)
		values ($1, $2, $3, $4, $5, now())
		returning created_at, expires_at`,
		id, userID, name, hash, *exp,
	).Scan(&createdAt, &storedExpires)
	if err != nil {
		return "", domain.APIToken{}, err
	}
	return plaintext, domain.APIToken{
		ID:        id,
		UserID:    userID,
		Name:      name,
		ExpiresAt: &storedExpires,
		CreatedAt: createdAt,
	}, nil
}

func (s *Store) CreateSessionAPIToken(ctx context.Context, userID string, ttl time.Duration) (string, domain.APIToken, error) {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	expires := time.Now().UTC().Add(ttl)
	return s.CreateUserAPIToken(ctx, userID, "__web_session", &expires)
}

func (s *Store) ListUserAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	rows, err := s.pool.Query(ctx, `
		select id, user_id, name, last_used_at, expires_at, created_at
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
		var lastUsed, expires *time.Time
		var created time.Time
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &lastUsed, &expires, &created); err != nil {
			return nil, err
		}
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
