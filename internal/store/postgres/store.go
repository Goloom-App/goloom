package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool      *pgxpool.Pool
	encrypter *security.Encrypter
}

func New(ctx context.Context, databaseURL string, encrypter *security.Encrypter) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool, encrypter: encrypter}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) UpsertOIDCUser(ctx context.Context, subject, email, name string) (domain.User, error) {
	const query = `
		insert into users (subject, email, name)
		values ($1, $2, $3)
		on conflict (subject) do update
		set email = excluded.email,
		    name = excluded.name,
		    updated_at = now()
		returning id, email, name, subject, created_at
	`

	var user domain.User
	err := s.pool.QueryRow(ctx, query, subject, email, name).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Subject,
		&user.CreatedAt,
	)
	return user, err
}

func (s *Store) LookupAPIToken(ctx context.Context, bearerToken string) (domain.AuthenticatedPrincipal, error) {
	const query = `
		select u.id, u.email, u.name, u.subject, u.created_at
		from api_tokens t
		join users u on u.id = t.user_id
		where t.token_hash = $1
		  and (t.expires_at is null or t.expires_at > now())
	`

	hash := security.HashToken(bearerToken)
	var principal domain.AuthenticatedPrincipal
	principal.Kind = "api_token"
	err := s.pool.QueryRow(ctx, query, hash).Scan(
		&principal.User.ID,
		&principal.User.Email,
		&principal.User.Name,
		&principal.User.Subject,
		&principal.User.CreatedAt,
	)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, err
	}

	_, _ = s.pool.Exec(ctx, `update api_tokens set last_used_at = now() where token_hash = $1`, hash)
	return principal, nil
}

func (s *Store) UserHasAnyTeamRole(ctx context.Context, userID, teamID string, roles ...domain.TeamRole) (bool, error) {
	const query = `select role from team_memberships where user_id = $1 and team_id = $2`
	var role domain.TeamRole
	if err := s.pool.QueryRow(ctx, query, userID, teamID).Scan(&role); err != nil {
		return false, err
	}
	for _, allowed := range roles {
		if role == allowed {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) ListTeamAccounts(ctx context.Context, teamID string) ([]domain.SocialAccount, error) {
	const query = `
		select id, team_id, provider, instance_url, username, remote_account_id,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, created_at
		from social_accounts
		where team_id = $1
		order by created_at desc
	`

	rows, err := s.pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []domain.SocialAccount
	for rows.Next() {
		var account domain.SocialAccount
		if err := rows.Scan(
			&account.ID,
			&account.TeamID,
			&account.Provider,
			&account.InstanceURL,
			&account.Username,
			&account.RemoteAccountID,
			&account.AccessTokenCiphertext,
			&account.RefreshTokenCiphertext,
			&account.MaxCharsOverride,
			&account.CreatedAt,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (s *Store) CreateAccount(ctx context.Context, teamID string, input domain.CreateAccountInput) (domain.SocialAccount, error) {
	accessCipher, err := s.encrypter.Encrypt(input.AccessToken)
	if err != nil {
		return domain.SocialAccount{}, fmt.Errorf("encrypt access token: %w", err)
	}

	refreshCipher := ""
	if input.RefreshToken != "" {
		refreshCipher, err = s.encrypter.Encrypt(input.RefreshToken)
		if err != nil {
			return domain.SocialAccount{}, fmt.Errorf("encrypt refresh token: %w", err)
		}
	}

	const query = `
		insert into social_accounts (
			team_id, provider, instance_url, username, remote_account_id,
			access_token_ciphertext, refresh_token_ciphertext
		)
		values ($1, $2, $3, $4, $5, $6, $7)
		returning id, team_id, provider, instance_url, username, remote_account_id,
		          access_token_ciphertext, refresh_token_ciphertext, max_chars_override, created_at
	`

	var account domain.SocialAccount
	err = s.pool.QueryRow(
		ctx,
		query,
		teamID,
		input.Provider,
		input.InstanceURL,
		input.Username,
		input.RemoteAccountID,
		accessCipher,
		refreshCipher,
	).Scan(
		&account.ID,
		&account.TeamID,
		&account.Provider,
		&account.InstanceURL,
		&account.Username,
		&account.RemoteAccountID,
		&account.AccessTokenCiphertext,
		&account.RefreshTokenCiphertext,
		&account.MaxCharsOverride,
		&account.CreatedAt,
	)
	return account, err
}

func (s *Store) GetAccountsByIDs(ctx context.Context, teamID string, ids []string) ([]domain.SocialAccount, error) {
	const query = `
		select id, team_id, provider, instance_url, username, remote_account_id,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, created_at
		from social_accounts
		where team_id = $1 and id = any($2)
	`

	rows, err := s.pool.Query(ctx, query, teamID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []domain.SocialAccount
	for rows.Next() {
		var account domain.SocialAccount
		if err := rows.Scan(
			&account.ID,
			&account.TeamID,
			&account.Provider,
			&account.InstanceURL,
			&account.Username,
			&account.RemoteAccountID,
			&account.AccessTokenCiphertext,
			&account.RefreshTokenCiphertext,
			&account.MaxCharsOverride,
			&account.CreatedAt,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (s *Store) CreateScheduledPost(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	defer tx.Rollback(ctx)

	const insertPost = `
		insert into scheduled_posts (team_id, author_user_id, content, scheduled_at, status)
		values ($1, $2, $3, $4, $5)
		returning id, team_id, author_user_id, content, scheduled_at, status,
		          attempt_count, coalesce(last_error, ''), created_at, updated_at
	`

	post := domain.ScheduledPost{}
	err = tx.QueryRow(ctx, insertPost, teamID, principal.User.ID, input.Content, input.ScheduledAt, domain.PostStatusPending).Scan(
		&post.ID,
		&post.TeamID,
		&post.AuthorUserID,
		&post.Content,
		&post.ScheduledAt,
		&post.Status,
		&post.AttemptCount,
		&post.LastError,
		&post.CreatedAt,
		&post.UpdatedAt,
	)
	if err != nil {
		return domain.ScheduledPost{}, err
	}

	post.TargetAccounts = make([]string, 0, len(input.TargetAccounts))
	for _, accountID := range input.TargetAccounts {
		if _, err := tx.Exec(
			ctx,
			`insert into scheduled_post_targets (post_id, account_id, status) values ($1, $2, $3)`,
			post.ID,
			accountID,
			domain.PostStatusPending,
		); err != nil {
			return domain.ScheduledPost{}, err
		}
		post.TargetAccounts = append(post.TargetAccounts, accountID)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.ScheduledPost{}, err
	}
	return post, nil
}

func (s *Store) ListTeamPosts(ctx context.Context, teamID string) ([]domain.ScheduledPost, error) {
	const query = `
		select p.id, p.team_id, p.author_user_id, p.content, p.scheduled_at, p.status,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       coalesce(array_agg(t.account_id::text) filter (where t.account_id is not null), '{}')
		from scheduled_posts p
		left join scheduled_post_targets t on t.post_id = p.id
		where p.team_id = $1
		group by p.id
		order by p.scheduled_at asc
	`
	return s.listPosts(ctx, query, teamID)
}

func (s *Store) GetScheduledPost(ctx context.Context, teamID, postID string) (domain.ScheduledPost, error) {
	const query = `
		select p.id, p.team_id, p.author_user_id, p.content, p.scheduled_at, p.status,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       coalesce(array_agg(t.account_id::text) filter (where t.account_id is not null), '{}')
		from scheduled_posts p
		left join scheduled_post_targets t on t.post_id = p.id
		where p.team_id = $1 and p.id = $2
		group by p.id
	`

	posts, err := s.listPosts(ctx, query, teamID, postID)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	if len(posts) == 0 {
		return domain.ScheduledPost{}, errors.New("post not found")
	}
	return posts[0], nil
}

func (s *Store) UpdateScheduledPost(ctx context.Context, teamID, postID string, input domain.CreatePostInput) (domain.ScheduledPost, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(
		ctx,
		`update scheduled_posts set content = $1, scheduled_at = $2, updated_at = now() where id = $3 and team_id = $4`,
		input.Content, input.ScheduledAt, postID, teamID,
	)
	if err != nil {
		return domain.ScheduledPost{}, err
	}

	if _, err := tx.Exec(ctx, `delete from scheduled_post_targets where post_id = $1`, postID); err != nil {
		return domain.ScheduledPost{}, err
	}

	for _, accountID := range input.TargetAccounts {
		if _, err := tx.Exec(
			ctx,
			`insert into scheduled_post_targets (post_id, account_id, status) values ($1, $2, $3)`,
			postID, accountID, domain.PostStatusPending,
		); err != nil {
			return domain.ScheduledPost{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.ScheduledPost{}, err
	}
	return s.GetScheduledPost(ctx, teamID, postID)
}

func (s *Store) CancelScheduledPost(ctx context.Context, teamID, postID string) error {
	_, err := s.pool.Exec(
		ctx,
		`update scheduled_posts set status = $1, updated_at = now() where id = $2 and team_id = $3`,
		domain.PostStatusCancelled, postID, teamID,
	)
	return err
}

func (s *Store) DeleteScheduledPost(ctx context.Context, teamID, postID string) error {
	_, err := s.pool.Exec(ctx, `delete from scheduled_posts where id = $1 and team_id = $2`, postID, teamID)
	return err
}

func (s *Store) ListDuePosts(ctx context.Context, limit int) ([]domain.ScheduledPost, error) {
	const query = `
		select p.id, p.team_id, p.author_user_id, p.content, p.scheduled_at, p.status,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       coalesce(array_agg(t.account_id::text) filter (where t.account_id is not null), '{}')
		from scheduled_posts p
		left join scheduled_post_targets t on t.post_id = p.id
		where p.scheduled_at <= now()
		  and p.status in ($1, $2)
		  and p.attempt_count < 5
		group by p.id
		order by p.scheduled_at asc
		limit $3
	`
	return s.listPosts(ctx, query, domain.PostStatusPending, domain.PostStatusFailed, limit)
}

func (s *Store) MarkPostProcessing(ctx context.Context, postID string) error {
	_, err := s.pool.Exec(
		ctx,
		`update scheduled_posts set status = $1, updated_at = now() where id = $2`,
		domain.PostStatusProcessing,
		postID,
	)
	return err
}

func (s *Store) MarkPostResult(ctx context.Context, postID string, attemptCount int, status domain.PostStatus, lastError string, nextAttempt *time.Time) error {
	query := `update scheduled_posts set attempt_count = $1, status = $2, last_error = $3, updated_at = now()`
	args := []any{attemptCount, status, lastError}

	if nextAttempt != nil {
		query += `, scheduled_at = $4 where id = $5`
		args = append(args, *nextAttempt, postID)
	} else {
		query += ` where id = $4`
		args = append(args, postID)
	}

	_, err := s.pool.Exec(ctx, query, args...)
	return err
}

func (s *Store) MarkPostTargetResult(ctx context.Context, postID, accountID string, status domain.PostStatus, publishedURL, lastError string) error {
	_, err := s.pool.Exec(
		ctx,
		`update scheduled_post_targets
		 set status = $1, published_url = nullif($2, ''), last_error = nullif($3, '')
		 where post_id = $4 and account_id = $5`,
		status, publishedURL, lastError, postID, accountID,
	)
	return err
}

func (s *Store) LoadPostTargets(ctx context.Context, postID string) ([]domain.SocialAccount, error) {
	const query = `
		select a.id, a.team_id, a.provider, a.instance_url, a.username, a.remote_account_id,
		       a.access_token_ciphertext, a.refresh_token_ciphertext, a.max_chars_override, a.created_at
		from scheduled_post_targets t
		join social_accounts a on a.id = t.account_id
		where t.post_id = $1
	`

	rows, err := s.pool.Query(ctx, query, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []domain.SocialAccount
	for rows.Next() {
		var account domain.SocialAccount
		if err := rows.Scan(
			&account.ID,
			&account.TeamID,
			&account.Provider,
			&account.InstanceURL,
			&account.Username,
			&account.RemoteAccountID,
			&account.AccessTokenCiphertext,
			&account.RefreshTokenCiphertext,
			&account.MaxCharsOverride,
			&account.CreatedAt,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (s *Store) DecryptAccessToken(account domain.SocialAccount) (string, error) {
	return s.encrypter.Decrypt(account.AccessTokenCiphertext)
}

func (s *Store) listPosts(ctx context.Context, query string, args ...any) ([]domain.ScheduledPost, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []domain.ScheduledPost
	for rows.Next() {
		var post domain.ScheduledPost
		if err := rows.Scan(
			&post.ID,
			&post.TeamID,
			&post.AuthorUserID,
			&post.Content,
			&post.ScheduledAt,
			&post.Status,
			&post.AttemptCount,
			&post.LastError,
			&post.CreatedAt,
			&post.UpdatedAt,
			&post.TargetAccounts,
		); err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, rows.Err()
}
