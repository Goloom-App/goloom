package postgres

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

func encodeMediaExcludeJSON(m map[string][]string) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func decodePostMediaExclude(raw string, dest *map[string][]string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		*dest = nil
		return nil
	}
	var m map[string][]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return err
	}
	*dest = m
	return nil
}

func encodeMediaIDsJSON(ids []string) (string, error) {
	ids = domain.NormalizeMediaIDs(ids)
	b, err := json.Marshal(ids)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodePostMediaIDs(raw string, dest *[]string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		*dest = nil
		return nil
	}
	return json.Unmarshal([]byte(raw), dest)
}

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

	const subject = domain.BootstrapAdminSubject
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

	if _, err := tx.Exec(ctx, `delete from api_tokens where user_id = $1 and name = $2`, userID, "Bootstrap admin token"); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		insert into api_tokens (user_id, name, token_hash, expires_at)
		values ($1, $2, $3, null)
	`, userID, "Bootstrap admin token", security.HashToken(token))
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	_, err = s.EnsurePersonalTeam(ctx, userID)
	return err
}

func (s *Store) UpsertOIDCUser(ctx context.Context, subject, email, name string) (domain.User, error) {
	const query = `
		with first_oidc_admin as (
			select (count(*) filter (where subject <> $4::text) = 0) as grant_admin from users
		)
		insert into users (subject, email, name, is_admin)
		select $1, $2, $3, grant_admin
		from first_oidc_admin
		on conflict (subject) do update
		set email = excluded.email,
		    name = excluded.name,
		    updated_at = now()
		returning id, email, name, subject, is_admin, created_at
	`

	var user domain.User
	err := s.pool.QueryRow(ctx, query, subject, email, name, domain.BootstrapAdminSubject).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Subject,
		&user.IsAdmin,
		&user.CreatedAt,
	)
	if err != nil {
		return user, err
	}
	if _, err := s.EnsurePersonalTeam(ctx, user.ID); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Store) LookupAPIToken(ctx context.Context, bearerToken string) (domain.AuthenticatedPrincipal, error) {
	const query = `
		select u.id, u.email, u.name, u.subject, u.is_admin, u.created_at, t.scopes, t.team_id, t.name
		from api_tokens t
		join users u on u.id = t.user_id
		where t.token_hash = $1
		  and (t.expires_at is null or t.expires_at > now())
	`

	hash := security.HashToken(bearerToken)
	var principal domain.AuthenticatedPrincipal
	principal.Kind = "api_token"
	var rawScopes string
	var teamID sql.NullString
	var tokenName sql.NullString
	err := s.pool.QueryRow(ctx, query, hash).Scan(
		&principal.User.ID,
		&principal.User.Email,
		&principal.User.Name,
		&principal.User.Subject,
		&principal.User.IsAdmin,
		&principal.User.CreatedAt,
		&rawScopes,
		&teamID,
		&tokenName,
	)
	if tokenName.Valid && tokenName.String == domain.WebSessionAPITokenName {
		principal.Kind = "oidc"
	}
	if err != nil {
		return domain.AuthenticatedPrincipal{}, err
	}
	principal.Scopes, err = parseTokenScopes(rawScopes)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, err
	}
	if teamID.Valid && strings.TrimSpace(teamID.String) != "" {
		principal.TokenTeamID = &teamID.String
	}

	_, _ = s.pool.Exec(ctx, `
		update api_tokens
		set last_used_at = now(),
		    expires_at = case when name = '__web_session' then now() + interval '12 hours' else expires_at end
		where token_hash = $1`, hash)
	return principal, nil
}

func parseTokenScopes(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return nil, fmt.Errorf("parse token scopes: %w", err)
	}
	return scopes, nil
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

	users := make([]domain.User, 0, 8)
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
	_ = isAdmin
	query := `
		select id, name, description, created_at, is_personal, is_ai_enabled, personal_for_user_id, scheduling_prefs
		from teams
	`
	args := []any{userID}
	query += ` where id in (select team_id from team_memberships where user_id = $1)`
	query += ` order by is_personal desc, name asc`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teams := make([]domain.Team, 0, 4)
	for rows.Next() {
		team, err := scanTeamRow(rows)
		if err != nil {
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
		returning id, name, description, created_at, is_ai_enabled
	`

	var team domain.Team
	if err := tx.QueryRow(ctx, insertTeam, input.Name, input.Description).Scan(
		&team.ID,
		&team.Name,
		&team.Description,
		&team.CreatedAt,
		&team.IsAIEnabled,
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

func (s *Store) UpdateTeam(ctx context.Context, teamID string, input domain.UpdateTeamInput) (domain.Team, error) {
	if input.SchedulingPreferences != nil {
		prefsJSON, err := domain.EncodeTeamSchedulingPrefsJSON(*input.SchedulingPreferences)
		if err != nil {
			return domain.Team{}, err
		}
		if input.IsAIEnabled != nil {
			const query = `
				update teams
				set name = $2, description = $3, scheduling_prefs = $4, is_ai_enabled = $5
				where id = $1
				returning id, name, description, created_at, is_personal, is_ai_enabled, personal_for_user_id, scheduling_prefs
			`
			return scanTeamRow(s.pool.QueryRow(ctx, query, teamID, input.Name, input.Description, prefsJSON, *input.IsAIEnabled))
		}
		const query = `
			update teams
			set name = $2, description = $3, scheduling_prefs = $4
			where id = $1
			returning id, name, description, created_at, is_personal, is_ai_enabled, personal_for_user_id, scheduling_prefs
		`
		return scanTeamRow(s.pool.QueryRow(ctx, query, teamID, input.Name, input.Description, prefsJSON))
	}
	if input.IsAIEnabled != nil {
		const query = `
			update teams
			set name = $2, description = $3, is_ai_enabled = $4
			where id = $1
			returning id, name, description, created_at, is_personal, is_ai_enabled, personal_for_user_id, scheduling_prefs
		`
		return scanTeamRow(s.pool.QueryRow(ctx, query, teamID, input.Name, input.Description, *input.IsAIEnabled))
	}
	const query = `
		update teams
		set name = $2, description = $3
		where id = $1
		returning id, name, description, created_at, is_personal, is_ai_enabled, personal_for_user_id, scheduling_prefs
	`
	return scanTeamRow(s.pool.QueryRow(ctx, query, teamID, input.Name, input.Description))
}

func scanTeamRow(row interface {
	Scan(dest ...any) error
}) (domain.Team, error) {
	var team domain.Team
	var personal sql.NullString
	var schedulingPrefs sql.NullString
	err := row.Scan(
		&team.ID,
		&team.Name,
		&team.Description,
		&team.CreatedAt,
		&team.IsPersonal,
		&team.IsAIEnabled,
		&personal,
		&schedulingPrefs,
	)
	if err != nil {
		return domain.Team{}, err
	}
	if personal.Valid {
		team.PersonalForUserID = personal.String
	}
	if schedulingPrefs.Valid && strings.TrimSpace(schedulingPrefs.String) != "" {
		prefs, err := domain.ParseTeamSchedulingPrefsJSON(schedulingPrefs.String)
		if err != nil {
			return domain.Team{}, err
		}
		team.SchedulingPrefs = prefs
	} else {
		team.SchedulingPrefs = domain.DefaultTeamSchedulingPreferences()
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

	memberships := make([]domain.TeamMembership, 0, 4)
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

	items := make([]domain.ProviderInstance, 0, 4)
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
	existing, err := s.GetProviderInstanceByID(ctx, instanceID)
	if err != nil {
		return domain.ProviderInstance{}, err
	}

	clientSecretCiphertext := existing.ClientSecretCiphertext
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

func (s *Store) DeleteProviderInstance(ctx context.Context, instanceID string) error {
	var linked int
	if err := s.pool.QueryRow(ctx, `select count(*)::int from social_accounts where provider_instance_id = $1`, instanceID).Scan(&linked); err != nil {
		return err
	}
	if linked > 0 {
		return fmt.Errorf("%w", domain.ErrProviderInstanceInUse)
	}
	tag, err := s.pool.Exec(ctx, `delete from provider_instances where id = $1`, instanceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w", domain.ErrProviderInstanceNotFound)
	}
	return nil
}

func (s *Store) UserHasAnyTeamRole(ctx context.Context, userID, teamID string, roles ...domain.TeamRole) (bool, error) {
	const query = `select role from team_memberships where user_id = $1 and team_id = $2`
	var role domain.TeamRole
	if err := s.pool.QueryRow(ctx, query, userID, teamID).Scan(&role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
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
		select id, team_id, name, provider, auth_type, coalesce(provider_instance_id::text, ''), instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts
		where team_id = $1
		order by created_at desc
	`

	rows, err := s.pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]domain.SocialAccount, 0, 4)
	for rows.Next() {
		var account domain.SocialAccount
		var accessExpires sql.NullTime
		if err := rows.Scan(
			&account.ID,
			&account.TeamID,
			&account.Name,
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
		if accessExpires.Valid {
			t := accessExpires.Time.UTC()
			account.AccessTokenExpiresAt = &t
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
			team_id, name, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
			avatar_url,
			access_token_ciphertext, refresh_token_ciphertext, access_token_expires_at
		)
		values ($1, '', $2, $3, nullif($4, '')::uuid, $5, $6, $7, $8, $9, $10, $11)
		returning id, team_id, name, provider, auth_type, coalesce(provider_instance_id::text, ''), instance_url, username, remote_account_id,
		          avatar_url,
		          access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
	`

	var account domain.SocialAccount
	var accessExpires sql.NullTime
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
		strings.TrimSpace(input.AvatarURL),
		accessCipher,
		refreshCipher,
		input.AccessTokenExpiresAt,
	).Scan(
		&account.ID,
		&account.TeamID,
		&account.Name,
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
	)
	if err != nil {
		return domain.SocialAccount{}, err
	}
	if accessExpires.Valid {
		t := accessExpires.Time.UTC()
		account.AccessTokenExpiresAt = &t
	}
	return account, nil
}

func (s *Store) UpdateAccount(ctx context.Context, teamID, accountID string, input domain.UpdateAccountInput) (domain.SocialAccount, error) {
	acc, err := s.GetAccountByID(ctx, accountID)
	if err != nil {
		return domain.SocialAccount{}, err
	}
	if acc.TeamID != teamID {
		return domain.SocialAccount{}, sql.ErrNoRows
	}

	name := acc.Name
	if input.Name != nil {
		name = *input.Name
	}
	maxChars := acc.MaxCharsOverride
	if input.MaxCharsOverride != nil {
		maxChars = input.MaxCharsOverride
	}

	accessCipher := acc.AccessTokenCiphertext
	if input.AccessToken != nil {
		accessCipher, err = s.encrypter.Encrypt(*input.AccessToken)
		if err != nil {
			return domain.SocialAccount{}, fmt.Errorf("encrypt access token: %w", err)
		}
	}
	refreshCipher := acc.RefreshTokenCiphertext
	if input.RefreshToken != nil {
		if *input.RefreshToken != "" {
			refreshCipher, err = s.encrypter.Encrypt(*input.RefreshToken)
			if err != nil {
				return domain.SocialAccount{}, fmt.Errorf("encrypt refresh token: %w", err)
			}
		} else {
			refreshCipher = ""
		}
	}

	const q = `
		update social_accounts set
			name = $1,
			max_chars_override = $2,
			access_token_ciphertext = $3,
			refresh_token_ciphertext = $4
		where id = $5
	`
	_, err = s.pool.Exec(ctx, q, name, maxChars, accessCipher, refreshCipher, accountID)
	if err != nil {
		return domain.SocialAccount{}, err
	}

	return s.GetAccountByID(ctx, accountID)
}

func (s *Store) DeleteAccount(ctx context.Context, teamID, accountID string) error {
	acc, err := s.GetAccountByID(ctx, accountID)
	if err != nil {
		return err
	}
	if acc.TeamID != teamID {
		return sql.ErrNoRows
	}
	return s.DeleteSocialAccount(ctx, accountID)
}

func (s *Store) GetAccountsByIDs(ctx context.Context, teamID string, ids []string) ([]domain.SocialAccount, error) {
	const query = `
		select id, team_id, name, provider, auth_type, coalesce(provider_instance_id::text, ''), instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts
		where team_id = $1 and id = any($2)
	`

	rows, err := s.pool.Query(ctx, query, teamID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]domain.SocialAccount, 0, len(ids))
	for rows.Next() {
		var account domain.SocialAccount
		var accessExpires sql.NullTime
		if err := rows.Scan(
			&account.ID,
			&account.TeamID,
			&account.Name,
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
		if accessExpires.Valid {
			t := accessExpires.Time.UTC()
			account.AccessTokenExpiresAt = &t
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

	visibility := domain.NormalizePostVisibility(input.Visibility)
	mediaJSON, err := encodeMediaIDsJSON(input.MediaIDs)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	excludeJSON, err := encodeMediaExcludeJSON(input.MediaExcludeByAccount)
	if err != nil {
		return domain.ScheduledPost{}, err
	}

	const insertPost = `
		insert into scheduled_posts (team_id, author_user_id, title, content, scheduled_at, status, source, visibility, media_ids, media_exclude_by_account, post_template_id, template_counter, template_occurrence_at, template_post_role, rss_feed_id)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		returning id, team_id, author_user_id, title, content, scheduled_at, status,
		          attempt_count, coalesce(last_error, ''), visibility, media_ids, media_exclude_by_account, created_at, updated_at, post_template_id, template_counter
	`

	authorID := principal.User.ID
	if input.AuthorUserID != nil && strings.TrimSpace(*input.AuthorUserID) != "" {
		authorID = strings.TrimSpace(*input.AuthorUserID)
	}
	var templateID any
	var templateCounter any
	var templateOccurrence any
	templateRole := strings.TrimSpace(input.TemplatePostRole)
	if input.PostTemplateID != nil && strings.TrimSpace(*input.PostTemplateID) != "" {
		templateID = strings.TrimSpace(*input.PostTemplateID)
	}
	if input.TemplateCounter != nil {
		templateCounter = *input.TemplateCounter
	}
	if input.TemplateOccurrenceAt != nil && !input.TemplateOccurrenceAt.IsZero() {
		templateOccurrence = input.TemplateOccurrenceAt.UTC()
	}
	source := input.Source
	if strings.TrimSpace(string(source)) == "" {
		source = domain.PostSourceScheduled
	}
	var rssFeedID any
	if input.RSSFeedID != nil && strings.TrimSpace(*input.RSSFeedID) != "" {
		rssFeedID = strings.TrimSpace(*input.RSSFeedID)
	}

	post := domain.ScheduledPost{}
	var mediaRaw, mediaExcludeRaw string
	var postTemplateID sql.NullString
	var templateCtr sql.NullInt64
	st := domain.PostStatusPending
	// Safety check: if the post is in the future, it must be pending or draft.
	if !input.Draft && input.ScheduledAt.After(time.Now().Add(5*time.Minute)) {
		st = domain.PostStatusPending
	} else if input.Draft {
		st = domain.PostStatusDraft
	}
	err = tx.QueryRow(ctx, insertPost, teamID, authorID, input.Title, input.Content, input.ScheduledAt, st, source, visibility, mediaJSON, excludeJSON, templateID, templateCounter, templateOccurrence, templateRole, rssFeedID).Scan(
		&post.ID,
		&post.TeamID,
		&post.AuthorUserID,
		&post.Title,
		&post.Content,
		&post.ScheduledAt,
		&post.Status,
		&post.AttemptCount,
		&post.LastError,
		&post.Visibility,
		&mediaRaw,
		&mediaExcludeRaw,
		&post.CreatedAt,
		&post.UpdatedAt,
		&postTemplateID,
		&templateCtr,
	)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	if postTemplateID.Valid {
		s := postTemplateID.String
		post.PostTemplateID = &s
	}
	if templateCtr.Valid {
		v := int(templateCtr.Int64)
		post.TemplateCounter = &v
	}
	if err := decodePostMediaIDs(mediaRaw, &post.MediaIDs); err != nil {
		return domain.ScheduledPost{}, err
	}
	if err := decodePostMediaExclude(mediaExcludeRaw, &post.MediaExcludeByAccount); err != nil {
		return domain.ScheduledPost{}, err
	}

	for accountID, content := range input.AccountContentOverride {
		if _, err := tx.Exec(ctx, `
			insert into post_versions (post_id, account_id, content)
			values ($1, $2, $3)`,
			post.ID, accountID, content,
		); err != nil {
			return domain.ScheduledPost{}, err
		}
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
		select p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status, p.source,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       p.visibility, p.media_ids, coalesce(p.media_exclude_by_account::text, '{}'),
		       p.post_template_id::text, p.template_counter,
		       coalesce(array_agg(t.account_id::text) filter (where t.account_id is not null), '{}')
		from scheduled_posts p
		left join scheduled_post_targets t on t.post_id = p.id
		where p.team_id = $1
		group by p.id
		order by p.scheduled_at asc
	`
	return s.listPosts(ctx, query, teamID)
}

func (s *Store) ListTeamPostsPage(ctx context.Context, teamID string, limit, offset int) ([]domain.ScheduledPost, int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx, `
		select count(*) from scheduled_posts where team_id = $1`,
		teamID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	const query = `
		select p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status, p.source,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       p.visibility, p.media_ids, coalesce(p.media_exclude_by_account::text, '{}'),
		       p.post_template_id::text, p.template_counter,
		       coalesce(array_agg(t.account_id::text) filter (where t.account_id is not null), '{}')
		from scheduled_posts p
		left join scheduled_post_targets t on t.post_id = p.id
		where p.team_id = $1
		group by p.id
		order by p.scheduled_at asc
		limit $2 offset $3
	`
	posts, err := s.listPosts(ctx, query, teamID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return posts, total, nil
}

func (s *Store) GetScheduledPost(ctx context.Context, teamID, postID string) (domain.ScheduledPost, error) {
	const query = `
		select p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status, p.source,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       p.visibility, p.media_ids, coalesce(p.media_exclude_by_account::text, '{}'),
		       p.post_template_id::text, p.template_counter,
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

func (s *Store) PatchScheduledPost(ctx context.Context, teamID, postID string, patch domain.UpdatePostPatch) (domain.ScheduledPost, error) {
	existing, err := s.GetScheduledPost(ctx, teamID, postID)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	versions, err := s.ListPostVersionsForTeamPost(ctx, teamID, postID)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	merged, flags := domain.ApplyPostPatch(existing, versions, patch)
	if !flags.Any() {
		return existing, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	defer tx.Rollback(ctx)

	postFieldChange := flags.Title || flags.Content || flags.ScheduledAt || flags.Visibility ||
		flags.MediaIDs || flags.MediaExcludeByAccount || flags.Draft
	if postFieldChange {
		visibility := domain.NormalizePostVisibility(merged.Visibility)
		mediaJSON, err := encodeMediaIDsJSON(merged.MediaIDs)
		if err != nil {
			return domain.ScheduledPost{}, err
		}
		excludeJSON, err := encodeMediaExcludeJSON(merged.MediaExcludeByAccount)
		if err != nil {
			return domain.ScheduledPost{}, err
		}
		newStatus := domain.ResolvePostStatusOnUpdate(existing.Status, existing.Source, merged)
		if flags.ScheduledAt && !flags.Title && !flags.Content && !flags.Visibility && !flags.MediaIDs && !flags.MediaExcludeByAccount && !flags.Draft {
			_, err = tx.Exec(ctx, `
				update scheduled_posts
				set scheduled_at = $1, status = $2, updated_at = now()
				where id = $3 and team_id = $4`,
				merged.ScheduledAt, string(newStatus), postID, teamID,
			)
		} else {
			_, err = tx.Exec(ctx, `
				update scheduled_posts
				set title = $1, content = $2, scheduled_at = $3, visibility = $4, media_ids = $5, media_exclude_by_account = $6, status = $7, updated_at = now()
				where id = $8 and team_id = $9`,
				merged.Title, merged.Content, merged.ScheduledAt, visibility, mediaJSON, excludeJSON, string(newStatus), postID, teamID,
			)
		}
		if err != nil {
			return domain.ScheduledPost{}, err
		}
	}

	if flags.Versions {
		if _, err := tx.Exec(ctx, `delete from post_versions where post_id = $1`, postID); err != nil {
			return domain.ScheduledPost{}, err
		}
		for accountID, content := range merged.AccountContentOverride {
			if _, err := tx.Exec(ctx, `
				insert into post_versions (post_id, account_id, content)
				values ($1, $2, $3)`,
				postID, accountID, content,
			); err != nil {
				return domain.ScheduledPost{}, err
			}
		}
	}

	if flags.TargetAccounts {
		if _, err := tx.Exec(ctx, `delete from scheduled_post_targets where post_id = $1`, postID); err != nil {
			return domain.ScheduledPost{}, err
		}
		for _, accountID := range merged.TargetAccounts {
			if _, err := tx.Exec(
				ctx,
				`insert into scheduled_post_targets (post_id, account_id, status) values ($1, $2, $3)`,
				postID, accountID, domain.PostStatusPending,
			); err != nil {
				return domain.ScheduledPost{}, err
			}
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

func (s *Store) GetScheduledPostTemplateLink(ctx context.Context, teamID, postID string) (string, *time.Time, string, error) {
	var tplID sql.NullString
	var occ sql.NullTime
	var role sql.NullString
	err := s.pool.QueryRow(ctx, `
		select post_template_id::text, template_occurrence_at, template_post_role
		from scheduled_posts
		where id = $1 and team_id = $2`,
		postID, teamID,
	).Scan(&tplID, &occ, &role)
	if err == sql.ErrNoRows {
		return "", nil, "", errors.New("post not found")
	}
	if err != nil {
		return "", nil, "", err
	}
	if !tplID.Valid || strings.TrimSpace(tplID.String) == "" {
		return "", nil, "", nil
	}
	var occAt *time.Time
	if occ.Valid {
		u := occ.Time.UTC()
		occAt = &u
	}
	return tplID.String, occAt, strings.TrimSpace(role.String), nil
}

func (s *Store) ListDuePosts(ctx context.Context, limit int) ([]domain.ScheduledPost, error) {
	const query = `
		select p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status, p.source,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       p.visibility, p.media_ids, coalesce(p.media_exclude_by_account::text, '{}'),
		       p.post_template_id::text, p.template_counter,
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

func (s *Store) MarkPostTargetResult(ctx context.Context, postID, accountID string, status domain.PostStatus, publishedURL, lastError string, publishMetadata map[string]string, remotePostID string) error {
	metaJSON := "{}"
	if publishMetadata != nil {
		b, err := json.Marshal(publishMetadata)
		if err != nil {
			return err
		}
		metaJSON = string(b)
	}
	_, err := s.pool.Exec(
		ctx,
		`update scheduled_post_targets
		 set status = $1, published_url = nullif($2, ''), last_error = nullif($3, ''), publish_metadata = $4,
		     remote_post_id = nullif($5, '')
		 where post_id = $6 and account_id = $7`,
		status, publishedURL, lastError, metaJSON, strings.TrimSpace(remotePostID), postID, accountID,
	)
	return err
}

func (s *Store) LoadPostTargets(ctx context.Context, postID string) ([]domain.SocialAccount, error) {
	const query = `
		select a.id, a.team_id, a.provider, a.auth_type, coalesce(a.provider_instance_id::text, ''), a.instance_url, a.username, a.remote_account_id,
		       a.avatar_url,
		       a.access_token_ciphertext, a.refresh_token_ciphertext, a.max_chars_override, a.access_token_expires_at, a.created_at
		from scheduled_post_targets t
		join social_accounts a on a.id = t.account_id
		where t.post_id = $1
	`

	rows, err := s.pool.Query(ctx, query, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]domain.SocialAccount, 0, 4)
	for rows.Next() {
		var account domain.SocialAccount
		var accessExpires sql.NullTime
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
		if accessExpires.Valid {
			t := accessExpires.Time.UTC()
			account.AccessTokenExpiresAt = &t
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

func (s *Store) UpdateSocialAccountTokens(ctx context.Context, accountID string, accessToken, refreshToken string, accessExpiresAt *time.Time) error {
	accessCipher, err := s.encrypter.Encrypt(accessToken)
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}
	if strings.TrimSpace(refreshToken) != "" {
		refreshCipher, encErr := s.encrypter.Encrypt(refreshToken)
		if encErr != nil {
			return fmt.Errorf("encrypt refresh token: %w", encErr)
		}
		_, err = s.pool.Exec(ctx, `
			update social_accounts
			set access_token_ciphertext = $1, refresh_token_ciphertext = $2, access_token_expires_at = $3
			where id = $4`,
			accessCipher, refreshCipher, accessExpiresAt, accountID,
		)
		return err
	}
	_, err = s.pool.Exec(ctx, `
		update social_accounts
		set access_token_ciphertext = $1, access_token_expires_at = $2
		where id = $3`,
		accessCipher, accessExpiresAt, accountID,
	)
	return err
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

	posts := make([]domain.ScheduledPost, 0, 16)
	for rows.Next() {
		var post domain.ScheduledPost
		var mediaRaw, mediaExcludeRaw string
		var postTemplateID sql.NullString
		var templateCtr sql.NullInt64
		if err := rows.Scan(
			&post.ID,
			&post.TeamID,
			&post.AuthorUserID,
			&post.Title,
			&post.Content,
			&post.ScheduledAt,
			&post.Status,
			&post.Source,
			&post.AttemptCount,
			&post.LastError,
			&post.CreatedAt,
			&post.UpdatedAt,
			&post.Visibility,
			&mediaRaw,
			&mediaExcludeRaw,
			&postTemplateID,
			&templateCtr,
			&post.TargetAccounts,
		); err != nil {
			return nil, err
		}
		if postTemplateID.Valid {
			s := postTemplateID.String
			post.PostTemplateID = &s
		}
		if templateCtr.Valid {
			v := int(templateCtr.Int64)
			post.TemplateCounter = &v
		}
		if strings.TrimSpace(post.Visibility) == "" {
			post.Visibility = domain.PostVisibilityPublic
		}
		if strings.TrimSpace(string(post.Source)) == "" {
			post.Source = domain.PostSourceScheduled
		}
		if err := decodePostMediaIDs(mediaRaw, &post.MediaIDs); err != nil {
			return nil, err
		}
		if err := decodePostMediaExclude(mediaExcludeRaw, &post.MediaExcludeByAccount); err != nil {
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
