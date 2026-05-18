package sqlite

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
	if err := s.db.QueryRowContext(ctx, `select count(*) from users`).Scan(&m.UsersCount); err != nil {
		return domain.AdminMetrics{}, err
	}
	if err := s.db.QueryRowContext(ctx, `select count(*) from teams`).Scan(&m.TeamsCount); err != nil {
		return domain.AdminMetrics{}, err
	}
	if err := s.db.QueryRowContext(ctx, `select count(*) from provider_instances`).Scan(&m.ProviderInstancesCount); err != nil {
		return domain.AdminMetrics{}, err
	}

	rows, err := s.db.QueryContext(ctx, `select status, count(*) from scheduled_posts group by status`)
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

func (s *Store) RepairFuturePostedPosts(ctx context.Context) (int64, error) {
	now := nowString()
	res, err := s.db.ExecContext(ctx, `
		update scheduled_posts
		set status = ?
		where status = ?
		  and scheduled_at > ?`,
		domain.PostStatusPending, domain.PostStatusPosted, now,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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
	expiresStr := exp.UTC().Format(time.RFC3339)
	plaintext, err := randomAPIToken()
	if err != nil {
		return "", domain.APIToken{}, err
	}
	hash := security.HashToken(plaintext)
	id := uuid.NewString()
	now := nowString()
	_, err = s.db.ExecContext(ctx, `
		insert into api_tokens (id, user_id, name, token_hash, expires_at, created_at)
		values (?, ?, ?, ?, ?, ?)`,
		id, userID, name, hash, expiresStr, now,
	)
	if err != nil {
		return "", domain.APIToken{}, err
	}
	created := mustParseTime(now)
	expParsed := mustParseTime(expiresStr)
	return plaintext, domain.APIToken{
		ID:        id,
		UserID:    userID,
		Name:      name,
		ExpiresAt: &expParsed,
		CreatedAt: created,
	}, nil
}

func (s *Store) TryAcquireLock(ctx context.Context, lockID string, duration time.Duration) (bool, error) {
	now := nowString()
	expiresAt := time.Now().UTC().Add(duration).Format(time.RFC3339)

	const query = `
		insert into job_locks (lock_id, locked_at, expires_at)
		values (?, ?, ?)
		on conflict (lock_id) do update
		set locked_at = excluded.locked_at, expires_at = excluded.expires_at
		where job_locks.expires_at < ?
		returning lock_id
	`
	var returnedID string
	err := s.db.QueryRowContext(ctx, query, lockID, now, expiresAt, now).Scan(&returnedID)
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
		ttl = 12 * time.Hour
	}
	expires := time.Now().UTC().Add(ttl)
	return s.CreateUserAPIToken(ctx, userID, domain.WebSessionAPITokenName, &expires)
}

func (s *Store) ListUserAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	now := nowString()
	if _, err := s.db.ExecContext(ctx, `
		delete from api_tokens
		where user_id = ?
		  and name = ?
		  and expires_at != ''
		  and expires_at <= ?`,
		userID, domain.WebSessionAPITokenName, now,
	); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		select id, user_id, name, last_used_at, expires_at, created_at
		from api_tokens
		where user_id = ?
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
		var lastUsed, expires sql.NullString
		var created string
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &lastUsed, &expires, &created); err != nil {
			return nil, err
		}
		if lastUsed.Valid && lastUsed.String != "" {
			parsed := mustParseTime(lastUsed.String)
			t.LastUsedAt = &parsed
		}
		if expires.Valid && expires.String != "" {
			parsed := mustParseTime(expires.String)
			t.ExpiresAt = &parsed
		}
		t.CreatedAt = mustParseTime(created)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeUserAPIToken(ctx context.Context, userID, tokenID string) error {
	res, err := s.db.ExecContext(ctx, `delete from api_tokens where id = ? and user_id = ?`, tokenID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
