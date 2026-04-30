package postgres

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

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
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply postgres schema: %w", err)
	}
	return &Store{pool: pool, encrypter: encrypter}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) EnsureBootstrapAdmin(ctx context.Context, email, name, token string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	const subject = "local-admin"
	var userID string
	err = tx.QueryRow(ctx, `
		insert into users (subject, email, name, is_admin)
		values ($1, $2, $3, true)
		on conflict (subject) do update
		set email = excluded.email,
		    name = excluded.name,
		    is_admin = true,
		    updated_at = now()
		returning id
	`, subject, email, name).Scan(&userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		insert into api_tokens (user_id, name, token_hash, expires_at)
		values ($1, $2, $3, null)
		on conflict (token_hash) do update
		set user_id = excluded.user_id,
		    name = excluded.name,
		    expires_at = null
	`, userID, "Bootstrap admin token", security.HashToken(token))
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) UpsertOIDCUser(ctx context.Context, subject, email, name string) (domain.User, error) {
	const query = `
		with first_user as (
			select count(*) = 0 as is_first from users
		)
		insert into users (subject, email, name, is_admin)
		select $1, $2, $3, is_first
		from first_user
		on conflict (subject) do update
		set email = excluded.email,
		    name = excluded.name,
		    updated_at = now()
		returning id, email, name, subject, is_admin, created_at
	`

	var user domain.User
	err := s.pool.QueryRow(ctx, query, subject, email, name).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Subject,
		&user.IsAdmin,
		&user.CreatedAt,
	)
	return user, err
}

func (s *Store) LookupAPIToken(ctx context.Context, bearerToken string) (domain.AuthenticatedPrincipal, error) {
	const query = `
		select u.id, u.email, u.name, u.subject, u.is_admin, u.created_at
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
		&principal.User.IsAdmin,
		&principal.User.CreatedAt,
	)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, err
	}

	_, _ = s.pool.Exec(ctx, `update api_tokens set last_used_at = now() where token_hash = $1`, hash)
	return principal, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	const query = `
		select id, email, name, subject, is_admin, created_at
		from users
		order by name asc, email asc
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.Name,
			&user.Subject,
			&user.IsAdmin,
			&user.CreatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) SetUserAdmin(ctx context.Context, userID string, isAdmin bool) (domain.User, error) {
	const query = `
		update users
		set is_admin = $1,
		    updated_at = now()
		where id = $2
		returning id, email, name, subject, is_admin, created_at
	`

	var user domain.User
	err := s.pool.QueryRow(ctx, query, isAdmin, userID).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Subject,
		&user.IsAdmin,
		&user.CreatedAt,
	)
	return user, err
}

func (s *Store) ListTeamsForUser(ctx context.Context, userID string, isAdmin bool) ([]domain.Team, error) {
	query := `
		select id, name, description, created_at
		from teams
	`
	args := make([]any, 0, 1)
	if !isAdmin {
		query += ` where id in (select team_id from team_memberships where user_id = $1)`
		args = append(args, userID)
	}
	query += ` order by name asc`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []domain.Team
	for rows.Next() {
		var team domain.Team
		if err := rows.Scan(&team.ID, &team.Name, &team.Description, &team.CreatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

func (s *Store) CreateTeam(ctx context.Context, ownerUserID string, input domain.CreateTeamInput) (domain.Team, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Team{}, err
	}
	defer tx.Rollback(ctx)

	const insertTeam = `
		insert into teams (name, description)
		values ($1, $2)
		returning id, name, description, created_at
	`

	var team domain.Team
	if err := tx.QueryRow(ctx, insertTeam, input.Name, input.Description).Scan(
		&team.ID,
		&team.Name,
		&team.Description,
		&team.CreatedAt,
	); err != nil {
		return domain.Team{}, err
	}

	if _, err := tx.Exec(
		ctx,
		`insert into team_memberships (user_id, team_id, role) values ($1, $2, $3)`,
		ownerUserID,
		team.ID,
		domain.RoleOwner,
	); err != nil {
		return domain.Team{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Team{}, err
	}
	return team, nil
}

func (s *Store) ListTeamMembers(ctx context.Context, teamID string) ([]domain.TeamMembership, error) {
	const query = `
		select user_id, team_id, role, created_at
		from team_memberships
		where team_id = $1
		order by created_at asc
	`

	rows, err := s.pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memberships []domain.TeamMembership
	for rows.Next() {
		var membership domain.TeamMembership
		if err := rows.Scan(
			&membership.UserID,
			&membership.TeamID,
			&membership.Role,
			&membership.CreatedAt,
		); err != nil {
			return nil, err
		}
		memberships = append(memberships, membership)
	}
	return memberships, rows.Err()
}

func (s *Store) AddTeamMember(ctx context.Context, teamID string, input domain.AddTeamMemberInput) (domain.TeamMembership, error) {
	const query = `
		insert into team_memberships (user_id, team_id, role)
		values ($1, $2, $3)
		on conflict (user_id, team_id) do update
		set role = excluded.role
		returning user_id, team_id, role, created_at
	`

	var membership domain.TeamMembership
	err := s.pool.QueryRow(ctx, query, input.UserID, teamID, input.Role).Scan(
		&membership.UserID,
		&membership.TeamID,
		&membership.Role,
		&membership.CreatedAt,
	)
	return membership, err
}

func (s *Store) RemoveTeamMember(ctx context.Context, teamID, userID string) error {
	_, err := s.pool.Exec(ctx, `delete from team_memberships where team_id = $1 and user_id = $2`, teamID, userID)
	return err
}

func (s *Store) ListProviderInstances(ctx context.Context, providerName string) ([]domain.ProviderInstance, error) {
	baseQuery := `
		select id, provider, name, instance_url, client_id, client_secret_ciphertext,
		       scopes, authorization_endpoint, token_endpoint, created_by_user_id,
		       created_at, updated_at
		from provider_instances
	`

	args := make([]any, 0, 1)
	if providerName != "" {
		baseQuery += ` where provider = $1`
		args = append(args, providerName)
	}
	baseQuery += ` order by provider asc, name asc, created_at asc`

	rows, err := s.pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.ProviderInstance
	for rows.Next() {
		instance, err := scanProviderInstance(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, instance)
	}
	return items, rows.Err()
}

func (s *Store) GetProviderInstanceByID(ctx context.Context, instanceID string) (domain.ProviderInstance, error) {
	const query = `
		select id, provider, name, instance_url, client_id, client_secret_ciphertext,
		       scopes, authorization_endpoint, token_endpoint, created_by_user_id,
		       created_at, updated_at
		from provider_instances
		where id = $1
	`

	return scanProviderInstance(s.pool.QueryRow(ctx, query, instanceID))
}

func (s *Store) CreateProviderInstance(ctx context.Context, createdByUserID string, input domain.PreparedProviderInstance) (domain.ProviderInstance, error) {
	clientSecretCiphertext := ""
	var err error
	if input.ClientSecret != "" {
		clientSecretCiphertext, err = s.encrypter.Encrypt(input.ClientSecret)
		if err != nil {
			return domain.ProviderInstance{}, fmt.Errorf("encrypt provider client secret: %w", err)
		}
	}

	const query = `
		insert into provider_instances (
			provider, name, instance_url, client_id, client_secret_ciphertext,
			scopes, authorization_endpoint, token_endpoint, created_by_user_id
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		returning id, provider, name, instance_url, client_id, client_secret_ciphertext,
		          scopes, authorization_endpoint, token_endpoint, created_by_user_id,
		          created_at, updated_at
	`

	return scanProviderInstance(s.pool.QueryRow(
		ctx,
		query,
		input.Provider,
		input.Name,
		input.InstanceURL,
		input.ClientID,
		clientSecretCiphertext,
		input.Scopes,
		input.AuthorizationEndpoint,
		input.TokenEndpoint,
		createdByUserID,
	))
}

func (s *Store) UpdateProviderInstance(ctx context.Context, instanceID string, input domain.PreparedProviderInstance) (domain.ProviderInstance, error) {
	clientSecretCiphertext := ""
	var err error
	if input.ClientSecret != "" {
		clientSecretCiphertext, err = s.encrypter.Encrypt(input.ClientSecret)
		if err != nil {
			return domain.ProviderInstance{}, fmt.Errorf("encrypt provider client secret: %w", err)
		}
	}

	const query = `
		update provider_instances
		set provider = $1,
		    name = $2,
		    instance_url = $3,
		    client_id = $4,
		    client_secret_ciphertext = $5,
		    scopes = $6,
		    authorization_endpoint = $7,
		    token_endpoint = $8,
		    updated_at = now()
		where id = $9
		returning id, provider, name, instance_url, client_id, client_secret_ciphertext,
		          scopes, authorization_endpoint, token_endpoint, created_by_user_id,
		          created_at, updated_at
	`

	return scanProviderInstance(s.pool.QueryRow(
		ctx,
		query,
		input.Provider,
		input.Name,
		input.InstanceURL,
		input.ClientID,
		clientSecretCiphertext,
		input.Scopes,
		input.AuthorizationEndpoint,
		input.TokenEndpoint,
		instanceID,
	))
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
		select id, team_id, provider, auth_type, coalesce(provider_instance_id::text, ''), instance_url, username, remote_account_id,
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
			&account.AuthType,
			&account.ProviderInstanceID,
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

func (s *Store) CreateAccount(ctx context.Context, teamID string, input domain.ConnectedAccount) (domain.SocialAccount, error) {
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
			team_id, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
			access_token_ciphertext, refresh_token_ciphertext
		)
		values ($1, $2, $3, nullif($4, '')::uuid, $5, $6, $7, $8, $9)
		returning id, team_id, provider, auth_type, coalesce(provider_instance_id::text, ''), instance_url, username, remote_account_id,
		          access_token_ciphertext, refresh_token_ciphertext, max_chars_override, created_at
	`

	var account domain.SocialAccount
	err = s.pool.QueryRow(
		ctx,
		query,
		teamID,
		input.Provider,
		input.AuthType,
		input.ProviderInstanceID,
		input.InstanceURL,
		input.Username,
		input.RemoteAccountID,
		accessCipher,
		refreshCipher,
	).Scan(
		&account.ID,
		&account.TeamID,
		&account.Provider,
		&account.AuthType,
		&account.ProviderInstanceID,
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

func (s *Store) DeleteAccount(ctx context.Context, teamID, accountID string) error {
	_, err := s.pool.Exec(ctx, `delete from social_accounts where id = $1 and team_id = $2`, accountID, teamID)
	return err
}

func (s *Store) GetAccountsByIDs(ctx context.Context, teamID string, ids []string) ([]domain.SocialAccount, error) {
	const query = `
		select id, team_id, provider, auth_type, coalesce(provider_instance_id::text, ''), instance_url, username, remote_account_id,
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
			&account.AuthType,
			&account.ProviderInstanceID,
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
		insert into scheduled_posts (team_id, author_user_id, title, content, scheduled_at, status)
		values ($1, $2, $3, $4, $5, $6)
		returning id, team_id, author_user_id, title, content, scheduled_at, status,
		          attempt_count, coalesce(last_error, ''), created_at, updated_at
	`

	post := domain.ScheduledPost{}
	err = tx.QueryRow(ctx, insertPost, teamID, principal.User.ID, input.Title, input.Content, input.ScheduledAt, domain.PostStatusPending).Scan(
		&post.ID,
		&post.TeamID,
		&post.AuthorUserID,
		&post.Title,
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
		select p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status,
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
		select p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status,
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
		`update scheduled_posts set title = $1, content = $2, scheduled_at = $3, updated_at = now() where id = $4 and team_id = $5`,
		input.Title, input.Content, input.ScheduledAt, postID, teamID,
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
		select p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status,
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
		select a.id, a.team_id, a.provider, a.auth_type, coalesce(a.provider_instance_id::text, ''), a.instance_url, a.username, a.remote_account_id,
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
			&account.AuthType,
			&account.ProviderInstanceID,
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

func (s *Store) DecryptRefreshToken(account domain.SocialAccount) (string, error) {
	if account.RefreshTokenCiphertext == "" {
		return "", nil
	}
	return s.encrypter.Decrypt(account.RefreshTokenCiphertext)
}

func (s *Store) DecryptProviderInstanceClientSecret(instance domain.ProviderInstance) (string, error) {
	if instance.ClientSecretCiphertext == "" {
		return "", nil
	}
	return s.encrypter.Decrypt(instance.ClientSecretCiphertext)
}

func (s *Store) LoadPublishedLinksByPostIDs(ctx context.Context, postIDs []string) (map[string]map[string]string, error) {
	if len(postIDs) == 0 {
		return map[string]map[string]string{}, nil
	}

	const query = `
		select post_id, account_id::text, published_url
		from scheduled_post_targets
		where post_id = any($1)
		  and published_url is not null
		  and published_url <> ''
	`

	rows, err := s.pool.Query(ctx, query, postIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := make(map[string]map[string]string, len(postIDs))
	for rows.Next() {
		var postID, accountID, publishedURL string
		if err := rows.Scan(&postID, &accountID, &publishedURL); err != nil {
			return nil, err
		}
		if links[postID] == nil {
			links[postID] = make(map[string]string)
		}
		links[postID][accountID] = publishedURL
	}
	return links, rows.Err()
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
			&post.Title,
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

type providerInstanceScanner interface {
	Scan(dest ...any) error
}

func scanProviderInstance(scanner providerInstanceScanner) (domain.ProviderInstance, error) {
	var instance domain.ProviderInstance
	err := scanner.Scan(
		&instance.ID,
		&instance.Provider,
		&instance.Name,
		&instance.InstanceURL,
		&instance.ClientID,
		&instance.ClientSecretCiphertext,
		&instance.Scopes,
		&instance.AuthorizationEndpoint,
		&instance.TokenEndpoint,
		&instance.CreatedByUserID,
		&instance.CreatedAt,
		&instance.UpdatedAt,
	)
	if err != nil {
		return domain.ProviderInstance{}, err
	}
	instance.HasClientSecret = instance.ClientSecretCiphertext != ""
	return instance, nil
}
