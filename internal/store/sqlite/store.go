package sqlite

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
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

const sqliteTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

type Store struct {
	db        *sql.DB
	encrypter *security.Encrypter
}

func New(ctx context.Context, databaseURL string, encrypter *security.Encrypter) (*Store, error) {
	db, err := sql.Open("sqlite", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	pragmas := []string{
		"pragma foreign_keys = on",
		"pragma journal_mode = wal",
		"pragma busy_timeout = 5000",
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure sqlite: %w", err)
		}
	}

	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply sqlite schema: %w", err)
	}

	if err := applySQLiteLegacyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("re-apply sqlite schema: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	return &Store{db: db, encrypter: encrypter}, nil
}

func (s *Store) Close() {
	_ = s.db.Close()
}

func (s *Store) EnsureBootstrapAdmin(ctx context.Context, email, name, token string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const subject = domain.BootstrapAdminSubject
	userID := uuid.NewString()
	now := nowString()
	_, err = tx.ExecContext(ctx, `
		insert into users (id, subject, email, name, is_admin, created_at, updated_at)
		values (?, ?, ?, ?, 1, ?, ?)
		on conflict(subject) do update set
			email = excluded.email,
			name = excluded.name,
			is_admin = 1,
			updated_at = excluded.updated_at`,
		userID, subject, email, name, now, now,
	)
	if err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `select id from users where subject = ?`, subject).Scan(&userID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `delete from api_tokens where user_id = ? and name = ?`, userID, "Bootstrap admin token"); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		insert into api_tokens (id, user_id, name, token_hash, expires_at, created_at)
		values (?, ?, ?, ?, null, ?)`,
		uuid.NewString(), userID, "Bootstrap admin token", security.HashToken(token), now,
	)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = s.EnsurePersonalTeam(ctx, userID)
	return err
}

func (s *Store) UpsertOIDCUser(ctx context.Context, subject, email, name string) (domain.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback()

	var existingID string
	err = tx.QueryRowContext(ctx, `select id from users where subject = ?`, subject).Scan(&existingID)
	switch {
	case err == nil:
		now := nowString()
		if _, err := tx.ExecContext(ctx,
			`update users set email = ?, name = ?, updated_at = ? where id = ?`,
			email, name, now, existingID,
		); err != nil {
			return domain.User{}, err
		}
		user, err := queryUser(ctx, tx, `select id, email, name, subject, is_admin, created_at from users where id = ?`, existingID)
		if err != nil {
			return domain.User{}, err
		}
		if err := tx.Commit(); err != nil {
			return domain.User{}, err
		}
		if _, err := s.EnsurePersonalTeam(ctx, user.ID); err != nil {
			return domain.User{}, err
		}
		return user, nil
	case !errors.Is(err, sql.ErrNoRows):
		return domain.User{}, err
	}

	var nonBootstrap int
	if err := tx.QueryRowContext(ctx, `select count(*) from users where subject != ?`, domain.BootstrapAdminSubject).Scan(&nonBootstrap); err != nil {
		return domain.User{}, err
	}
	isFirst := nonBootstrap == 0

	now := nowString()
	user := domain.User{
		ID:        uuid.NewString(),
		Email:     email,
		Name:      name,
		Subject:   subject,
		IsAdmin:   isFirst,
		CreatedAt: mustParseTime(now),
	}
	if _, err := tx.ExecContext(ctx, `
		insert into users (id, subject, email, name, is_admin, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?)`,
		user.ID, subject, email, name, boolToInt(isFirst), now, now,
	); err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.User{}, err
	}
	if _, err := s.EnsurePersonalTeam(ctx, user.ID); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Store) LookupAPIToken(ctx context.Context, bearerToken string) (domain.AuthenticatedPrincipal, error) {
	hash := security.HashToken(bearerToken)
	principal := domain.AuthenticatedPrincipal{Kind: "api_token"}
	now := nowString()

	var createdAt string
	var rawScopes string
	var teamID sql.NullString
	var tokenName sql.NullString
	row := s.db.QueryRowContext(ctx, `
		select u.id, u.email, u.name, u.subject, u.is_admin, u.created_at, t.scopes, t.team_id, t.name
		from api_tokens t
		join users u on u.id = t.user_id
		where t.token_hash = ?
		  and (t.expires_at is null or t.expires_at = '' or t.expires_at > ?)`,
		hash, now,
	)
	err := row.Scan(
		&principal.User.ID,
		&principal.User.Email,
		&principal.User.Name,
		&principal.User.Subject,
		&principal.User.IsAdmin,
		&createdAt,
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
	principal.User.CreatedAt = mustParseTime(createdAt)
	principal.Scopes, err = parseTokenScopes(rawScopes)
	if err != nil {
		return domain.AuthenticatedPrincipal{}, err
	}
	if teamID.Valid && strings.TrimSpace(teamID.String) != "" {
		principal.TokenTeamID = &teamID.String
	}

	rollingExpiry := formatTime(time.Now().UTC().Add(12 * time.Hour))
	_, _ = s.db.ExecContext(ctx, `
		update api_tokens
		set last_used_at = ?,
		    expires_at = case when name = '__web_session' then ? else expires_at end
		where token_hash = ?`,
		now, rollingExpiry, hash,
	)
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
	rows, err := s.db.QueryContext(ctx, `
		select id, email, name, subject, is_admin, created_at
		from users
		order by name asc, email asc`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectUsers(rows)
}

func (s *Store) SetUserAdmin(ctx context.Context, userID string, isAdmin bool) (domain.User, error) {
	_, err := s.db.ExecContext(ctx, `update users set is_admin = ?, updated_at = ? where id = ?`, boolToInt(isAdmin), nowString(), userID)
	if err != nil {
		return domain.User{}, err
	}
	return queryUser(ctx, s.db, `select id, email, name, subject, is_admin, created_at from users where id = ?`, userID)
}

func (s *Store) ListTeamsForUser(ctx context.Context, userID string, isAdmin bool) ([]domain.Team, error) {
	_ = isAdmin
	query := `
		select id, name, description, created_at, is_personal, is_ai_enabled, personal_for_user_id, scheduling_prefs, brand_color
		from teams
	`
	args := []any{userID}
	query += ` where id in (select team_id from team_memberships where user_id = ?)`
	query += ` order by is_personal desc, name asc`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectTeams(rows)
}

func (s *Store) CreateTeam(ctx context.Context, ownerUserID string, input domain.CreateTeamInput) (domain.Team, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Team{}, err
	}
	defer tx.Rollback()

	now := nowString()
	team := domain.Team{
		ID:          uuid.NewString(),
		Name:        input.Name,
		Description: input.Description,
		CreatedAt:   mustParseTime(now),
	}
	if _, err := tx.ExecContext(ctx, `
		insert into teams (id, name, description, is_personal, personal_for_user_id, created_at)
		values (?, ?, ?, 0, null, ?)`,
		team.ID, team.Name, team.Description, now,
	); err != nil {
		return domain.Team{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		insert into team_memberships (user_id, team_id, role, created_at)
		values (?, ?, ?, ?)`,
		ownerUserID, team.ID, domain.RoleOwner, now,
	); err != nil {
		return domain.Team{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Team{}, err
	}
	return team, nil
}

func (s *Store) UpdateTeam(ctx context.Context, teamID string, input domain.UpdateTeamInput) (domain.Team, error) {
	if input.BrandColor != nil {
		if _, err := s.db.ExecContext(ctx, `update teams set brand_color = ? where id = ?`, *input.BrandColor, teamID); err != nil {
			return domain.Team{}, err
		}
	}
	if input.SchedulingPreferences != nil {
		prefsJSON, err := domain.EncodeTeamSchedulingPrefsJSON(*input.SchedulingPreferences)
		if err != nil {
			return domain.Team{}, err
		}
		if input.IsAIEnabled != nil {
			_, err = s.db.ExecContext(ctx, `
				update teams
				set name = ?, description = ?, scheduling_prefs = ?, is_ai_enabled = ?
				where id = ?`,
				input.Name, input.Description, prefsJSON, boolToInt(*input.IsAIEnabled), teamID,
			)
		} else {
			_, err = s.db.ExecContext(ctx, `
				update teams
				set name = ?, description = ?, scheduling_prefs = ?
				where id = ?`,
				input.Name, input.Description, prefsJSON, teamID,
			)
		}
		if err != nil {
			return domain.Team{}, err
		}
	} else if input.IsAIEnabled != nil {
		_, err := s.db.ExecContext(ctx, `
			update teams
			set name = ?, description = ?, is_ai_enabled = ?
			where id = ?`,
			input.Name, input.Description, boolToInt(*input.IsAIEnabled), teamID,
		)
		if err != nil {
			return domain.Team{}, err
		}
	} else {
		_, err := s.db.ExecContext(ctx, `
			update teams
			set name = ?, description = ?
			where id = ?`,
			input.Name, input.Description, teamID,
		)
		if err != nil {
			return domain.Team{}, err
		}
	}
	return queryTeam(ctx, s.db, `
		select id, name, description, created_at, is_personal, is_ai_enabled, personal_for_user_id, scheduling_prefs, brand_color
		from teams
		where id = ?`,
		teamID,
	)
}

func (s *Store) ListTeamMembers(ctx context.Context, teamID string) ([]domain.TeamMembership, error) {
	rows, err := s.db.QueryContext(ctx, `
		select user_id, team_id, role, created_at
		from team_memberships
		where team_id = ?
		order by created_at asc`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectMemberships(rows)
}

func (s *Store) AddTeamMember(ctx context.Context, teamID string, input domain.AddTeamMemberInput) (domain.TeamMembership, error) {
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		insert into team_memberships (user_id, team_id, role, created_at)
		values (?, ?, ?, ?)
		on conflict(user_id, team_id) do update set role = excluded.role`,
		input.UserID, teamID, input.Role, now,
	)
	if err != nil {
		return domain.TeamMembership{}, err
	}
	return queryMembership(ctx, s.db, `
		select user_id, team_id, role, created_at
		from team_memberships
		where user_id = ? and team_id = ?`,
		input.UserID, teamID,
	)
}

func (s *Store) RemoveTeamMember(ctx context.Context, teamID, userID string) error {
	_, err := s.db.ExecContext(ctx, `delete from team_memberships where team_id = ? and user_id = ?`, teamID, userID)
	return err
}

func (s *Store) ListProviderInstances(ctx context.Context, providerName string) ([]domain.ProviderInstance, error) {
	query := `
		select id, provider, name, instance_url, client_id, client_secret_ciphertext,
		       scopes_json, authorization_endpoint, token_endpoint, created_by_user_id,
		       created_at, updated_at
		from provider_instances
	`
	args := []any{}
	if providerName != "" {
		query += ` where provider = ?`
		args = append(args, providerName)
	}
	query += ` order by provider asc, name asc, created_at asc`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectProviderInstances(rows)
}

func (s *Store) GetProviderInstanceByID(ctx context.Context, instanceID string) (domain.ProviderInstance, error) {
	return queryProviderInstance(ctx, s.db, `
		select id, provider, name, instance_url, client_id, client_secret_ciphertext,
		       scopes_json, authorization_endpoint, token_endpoint, created_by_user_id,
		       created_at, updated_at
		from provider_instances
		where id = ?`,
		instanceID,
	)
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

	now := nowString()
	instanceID := uuid.NewString()
	scopesJSON, err := marshalScopes(input.Scopes)
	if err != nil {
		return domain.ProviderInstance{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		insert into provider_instances (
			id, provider, name, instance_url, client_id, client_secret_ciphertext,
			scopes_json, authorization_endpoint, token_endpoint, created_by_user_id,
			created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		instanceID, input.Provider, input.Name, input.InstanceURL, input.ClientID, clientSecretCiphertext,
		scopesJSON, input.AuthorizationEndpoint, input.TokenEndpoint, createdByUserID, now, now,
	)
	if err != nil {
		return domain.ProviderInstance{}, err
	}
	return s.GetProviderInstanceByID(ctx, instanceID)
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

	scopesJSON, err := marshalScopes(input.Scopes)
	if err != nil {
		return domain.ProviderInstance{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		update provider_instances
		set provider = ?,
		    name = ?,
		    instance_url = ?,
		    client_id = ?,
		    client_secret_ciphertext = ?,
		    scopes_json = ?,
		    authorization_endpoint = ?,
		    token_endpoint = ?,
		    updated_at = ?
		where id = ?`,
		input.Provider, input.Name, input.InstanceURL, input.ClientID, clientSecretCiphertext, scopesJSON,
		input.AuthorizationEndpoint, input.TokenEndpoint, nowString(), instanceID,
	)
	if err != nil {
		return domain.ProviderInstance{}, err
	}
	return s.GetProviderInstanceByID(ctx, instanceID)
}

func (s *Store) DeleteProviderInstance(ctx context.Context, instanceID string) error {
	var linked int
	if err := s.db.QueryRowContext(ctx, `select count(*) from social_accounts where provider_instance_id = ?`, instanceID).Scan(&linked); err != nil {
		return err
	}
	if linked > 0 {
		return fmt.Errorf("%w", domain.ErrProviderInstanceInUse)
	}
	res, err := s.db.ExecContext(ctx, `delete from provider_instances where id = ?`, instanceID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w", domain.ErrProviderInstanceNotFound)
	}
	return nil
}

func (s *Store) UserHasAnyTeamRole(ctx context.Context, userID, teamID string, roles ...domain.TeamRole) (bool, error) {
	var role domain.TeamRole
	if err := s.db.QueryRowContext(ctx, `select role from team_memberships where user_id = ? and team_id = ?`, userID, teamID).Scan(&role); err != nil {
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
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, name, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts
		where team_id = ?
		order by created_at desc`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectAccounts(rows)
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

	accountID := uuid.NewString()
	now := nowString()
	var accessExpires any
	if input.AccessTokenExpiresAt != nil && !input.AccessTokenExpiresAt.IsZero() {
		accessExpires = formatTime(*input.AccessTokenExpiresAt)
	}
	_, err = s.db.ExecContext(ctx, `
		insert into social_accounts (
			id, team_id, name, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
			avatar_url,
			access_token_ciphertext, refresh_token_ciphertext, access_token_expires_at, created_at
		)
		values (?, ?, '', ?, ?, nullif(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, teamID, input.Provider, input.AuthType, input.ProviderInstanceID, input.InstanceURL, input.Username,
		input.RemoteAccountID, strings.TrimSpace(input.AvatarURL), accessCipher, refreshCipher, accessExpires, now,
	)
	if err != nil {
		return domain.SocialAccount{}, err
	}
	return queryAccount(ctx, s.db, `
		select id, team_id, name, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts
		where id = ?`,
		accountID,
	)
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

	_, err = s.db.ExecContext(ctx, `
		update social_accounts set
			name = ?,
			max_chars_override = ?,
			access_token_ciphertext = ?,
			refresh_token_ciphertext = ?
		where id = ?`,
		name, maxChars, accessCipher, refreshCipher, accountID,
	)
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
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders, args := inClause(ids)
	args = append([]any{teamID}, args...)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		select id, team_id, name, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts
		where team_id = ? and id in (%s)`, placeholders),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectAccounts(rows)
}

func (s *Store) CreateScheduledPost(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	defer tx.Rollback()

	now := nowString()
	postID := uuid.NewString()
	visibility := domain.NormalizePostVisibility(input.Visibility)
	mediaJSON, err := encodeMediaIDsJSON(input.MediaIDs)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	excludeJSON, err := encodeMediaExcludeJSON(input.MediaExcludeByAccount)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	st := domain.PostStatusPending
	// Safety check: if the post is in the future, it must be pending or draft.
	if !input.Draft && input.ScheduledAt.After(time.Now().Add(5*time.Minute)) {
		st = domain.PostStatusPending
	} else if input.Draft {
		st = domain.PostStatusDraft
	}
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
		templateOccurrence = formatTime(input.TemplateOccurrenceAt.UTC())
	}
	source := input.Source
	if strings.TrimSpace(string(source)) == "" {
		source = domain.PostSourceScheduled
	}
	var rssFeedID any
	if input.RSSFeedID != nil && strings.TrimSpace(*input.RSSFeedID) != "" {
		rssFeedID = strings.TrimSpace(*input.RSSFeedID)
	}

	if _, err := tx.ExecContext(ctx, `
		insert into scheduled_posts (
			id, team_id, author_user_id, title, content, scheduled_at, status, source,
			attempt_count, last_error, visibility, media_ids, media_exclude_by_account,
			post_template_id, template_counter, template_occurrence_at, template_post_role, rss_feed_id, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, 0, null, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		postID, teamID, authorID, input.Title, input.Content, formatTime(input.ScheduledAt), st, source,
		visibility, mediaJSON, excludeJSON, templateID, templateCounter, templateOccurrence, templateRole, rssFeedID, now, now,
	); err != nil {
		return domain.ScheduledPost{}, err
	}

	for accountID, content := range input.AccountContentOverride {
		if _, err := tx.ExecContext(ctx, `
			insert into post_versions (post_id, account_id, content)
			values (?, ?, ?)`,
			postID, accountID, content,
		); err != nil {
			return domain.ScheduledPost{}, err
		}
	}

	for _, accountID := range input.TargetAccounts {
		if _, err := tx.ExecContext(ctx, `
			insert into scheduled_post_targets (post_id, account_id, status)
			values (?, ?, ?)`,
			postID, accountID, domain.PostStatusPending,
		); err != nil {
			return domain.ScheduledPost{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.ScheduledPost{}, err
	}
	return s.GetScheduledPost(ctx, teamID, postID)
}

func (s *Store) ListTeamPosts(ctx context.Context, teamID string) ([]domain.ScheduledPost, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, title, content, scheduled_at, status, source,
		       attempt_count, last_error, visibility, media_ids, media_exclude_by_account,
		       post_template_id, template_counter, created_at, updated_at
		from scheduled_posts
		where team_id = ?
		order by scheduled_at asc`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posts, err := collectPosts(rows)
	if err != nil {
		return nil, err
	}
	if err := s.attachTargetAccounts(ctx, posts); err != nil {
		return nil, err
	}
	return posts, nil
}

func (s *Store) ListTeamPostsPage(ctx context.Context, teamID string, limit, offset int) ([]domain.ScheduledPost, int64, error) {
	var total int64
	err := s.db.QueryRowContext(ctx, `
		select count(*) from scheduled_posts where team_id = ?`,
		teamID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, title, content, scheduled_at, status, source,
		       attempt_count, last_error, visibility, media_ids, media_exclude_by_account,
		       post_template_id, template_counter, created_at, updated_at
		from scheduled_posts
		where team_id = ?
		order by scheduled_at asc
		limit ? offset ?`,
		teamID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	posts, err := collectPosts(rows)
	if err != nil {
		return nil, 0, err
	}
	if err := s.attachTargetAccounts(ctx, posts); err != nil {
		return nil, 0, err
	}
	return posts, total, nil
}

func (s *Store) GetScheduledPost(ctx context.Context, teamID, postID string) (domain.ScheduledPost, error) {
	post, err := queryPost(ctx, s.db, `
		select id, team_id, author_user_id, title, content, scheduled_at, status, source,
		       attempt_count, last_error, visibility, media_ids, media_exclude_by_account,
		       post_template_id, template_counter, created_at, updated_at
		from scheduled_posts
		where team_id = ? and id = ?`,
		teamID, postID,
	)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	targetsByPostID, err := s.loadTargetAccountIDs(ctx, []string{post.ID})
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	post.TargetAccounts = targetsByPostID[post.ID]
	return post, nil
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	defer tx.Rollback()

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
			if _, err := tx.ExecContext(ctx, `
				update scheduled_posts
				set scheduled_at = ?, status = ?, updated_at = ?
				where id = ? and team_id = ?`,
				formatTime(merged.ScheduledAt), string(newStatus), nowString(), postID, teamID,
			); err != nil {
				return domain.ScheduledPost{}, err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `
				update scheduled_posts
				set title = ?, content = ?, scheduled_at = ?, visibility = ?, media_ids = ?, media_exclude_by_account = ?, status = ?, updated_at = ?
				where id = ? and team_id = ?`,
				merged.Title, merged.Content, formatTime(merged.ScheduledAt), visibility, mediaJSON, excludeJSON, string(newStatus), nowString(), postID, teamID,
			); err != nil {
				return domain.ScheduledPost{}, err
			}
		}
	}

	if flags.Versions {
		if _, err := tx.ExecContext(ctx, `delete from post_versions where post_id = ?`, postID); err != nil {
			return domain.ScheduledPost{}, err
		}
		for accountID, content := range merged.AccountContentOverride {
			if _, err := tx.ExecContext(ctx, `
				insert into post_versions (post_id, account_id, content)
				values (?, ?, ?)`,
				postID, accountID, content,
			); err != nil {
				return domain.ScheduledPost{}, err
			}
		}
	}

	if flags.TargetAccounts {
		if _, err := tx.ExecContext(ctx, `delete from scheduled_post_targets where post_id = ?`, postID); err != nil {
			return domain.ScheduledPost{}, err
		}
		for _, accountID := range merged.TargetAccounts {
			if _, err := tx.ExecContext(ctx, `
				insert into scheduled_post_targets (post_id, account_id, status)
				values (?, ?, ?)`,
				postID, accountID, domain.PostStatusPending,
			); err != nil {
				return domain.ScheduledPost{}, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.ScheduledPost{}, err
	}
	return s.GetScheduledPost(ctx, teamID, postID)
}

func (s *Store) CancelScheduledPost(ctx context.Context, teamID, postID string) error {
	_, err := s.db.ExecContext(ctx, `
		update scheduled_posts
		set status = ?, updated_at = ?
		where id = ? and team_id = ?`,
		domain.PostStatusCancelled, nowString(), postID, teamID,
	)
	return err
}

func (s *Store) DeleteScheduledPost(ctx context.Context, teamID, postID string) error {
	_, err := s.db.ExecContext(ctx, `delete from scheduled_posts where id = ? and team_id = ?`, postID, teamID)
	return err
}

func (s *Store) GetScheduledPostTemplateLink(ctx context.Context, teamID, postID string) (string, *time.Time, string, error) {
	var tplID sql.NullString
	var occStr sql.NullString
	var role sql.NullString
	err := s.db.QueryRowContext(ctx, `
		select post_template_id, template_occurrence_at, template_post_role
		from scheduled_posts
		where id = ? and team_id = ?`,
		postID, teamID,
	).Scan(&tplID, &occStr, &role)
	if err == sql.ErrNoRows {
		return "", nil, "", errors.New("post not found")
	}
	if err != nil {
		return "", nil, "", err
	}
	if !tplID.Valid || strings.TrimSpace(tplID.String) == "" {
		return "", nil, "", nil
	}
	var occ *time.Time
	if occStr.Valid && strings.TrimSpace(occStr.String) != "" {
		parsed, err := parseTime(occStr.String)
		if err != nil {
			return "", nil, "", err
		}
		u := parsed.UTC()
		occ = &u
	}
	return tplID.String, occ, strings.TrimSpace(role.String), nil
}

func (s *Store) ListDuePosts(ctx context.Context, limit int) ([]domain.ScheduledPost, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, title, content, scheduled_at, status, source,
		       attempt_count, last_error, visibility, media_ids, media_exclude_by_account,
		       post_template_id, template_counter, created_at, updated_at
		from scheduled_posts
		where scheduled_at <= ?
		  and status in (?, ?)
		  and attempt_count < 5
		order by scheduled_at asc
		limit ?`,
		nowString(), domain.PostStatusPending, domain.PostStatusFailed, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posts, err := collectPosts(rows)
	if err != nil {
		return nil, err
	}
	if err := s.attachTargetAccounts(ctx, posts); err != nil {
		return nil, err
	}
	return posts, nil
}

func (s *Store) MarkPostProcessing(ctx context.Context, postID string) error {
	_, err := s.db.ExecContext(ctx, `update scheduled_posts set status = ?, updated_at = ? where id = ?`, domain.PostStatusProcessing, nowString(), postID)
	return err
}

func (s *Store) MarkPostResult(ctx context.Context, postID string, attemptCount int, status domain.PostStatus, lastError string, nextAttempt *time.Time) error {
	if nextAttempt != nil {
		_, err := s.db.ExecContext(ctx, `
			update scheduled_posts
			set attempt_count = ?, status = ?, last_error = ?, scheduled_at = ?, updated_at = ?
			where id = ?`,
			attemptCount, status, nullableString(lastError), formatTime(*nextAttempt), nowString(), postID,
		)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		update scheduled_posts
		set attempt_count = ?, status = ?, last_error = ?, updated_at = ?
		where id = ?`,
		attemptCount, status, nullableString(lastError), nowString(), postID,
	)
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
	_, err := s.db.ExecContext(ctx, `
		update scheduled_post_targets
		set status = ?, published_url = ?, last_error = ?, publish_metadata = ?, remote_post_id = ?
		where post_id = ? and account_id = ?`,
		status, nullableString(publishedURL), nullableString(lastError), metaJSON, nullableString(remotePostID), postID, accountID,
	)
	return err
}

func (s *Store) UpdateSocialAccountTokens(ctx context.Context, accountID string, accessToken, refreshToken string, accessExpiresAt *time.Time) error {
	accessCipher, err := s.encrypter.Encrypt(accessToken)
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}
	var exp any
	if accessExpiresAt != nil && !accessExpiresAt.IsZero() {
		exp = formatTime(*accessExpiresAt)
	}
	if strings.TrimSpace(refreshToken) != "" {
		refreshCipher, encErr := s.encrypter.Encrypt(refreshToken)
		if encErr != nil {
			return fmt.Errorf("encrypt refresh token: %w", encErr)
		}
		_, err = s.db.ExecContext(ctx, `
			update social_accounts
			set access_token_ciphertext = ?, refresh_token_ciphertext = ?, access_token_expires_at = ?
			where id = ?`,
			accessCipher, refreshCipher, exp, accountID,
		)
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		update social_accounts
		set access_token_ciphertext = ?, access_token_expires_at = ?
		where id = ?`,
		accessCipher, exp, accountID,
	)
	return err
}

func (s *Store) LoadPostTargets(ctx context.Context, postID string) ([]domain.SocialAccount, error) {
	rows, err := s.db.QueryContext(ctx, `
		select a.id, a.team_id, a.name, a.provider, a.auth_type, a.provider_instance_id, a.instance_url, a.username, a.remote_account_id,
		       a.avatar_url,
		       a.access_token_ciphertext, a.refresh_token_ciphertext, a.max_chars_override, a.access_token_expires_at, a.created_at
		from scheduled_post_targets t
		join social_accounts a on a.id = t.account_id
		where t.post_id = ?`,
		postID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectAccounts(rows)
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
	placeholders, args := inClause(postIDs)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		select post_id, account_id, published_url
		from scheduled_post_targets
		where post_id in (%s)
		  and published_url is not null
		  and published_url <> ''`, placeholders),
		args...,
	)
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

func (s *Store) attachTargetAccounts(ctx context.Context, posts []domain.ScheduledPost) error {
	if len(posts) == 0 {
		return nil
	}
	postIDs := make([]string, 0, len(posts))
	for i, post := range posts {
		post.TargetAccounts = []string{}
		postIDs = append(postIDs, post.ID)
		posts[i].TargetAccounts = []string{}
	}

	targetsByPostID, err := s.loadTargetAccountIDs(ctx, postIDs)
	if err != nil {
		return err
	}
	for i := range posts {
		posts[i].TargetAccounts = targetsByPostID[posts[i].ID]
	}
	return nil
}

func (s *Store) loadTargetAccountIDs(ctx context.Context, postIDs []string) (map[string][]string, error) {
	placeholders, args := inClause(postIDs)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		select post_id, account_id
		from scheduled_post_targets
		where post_id in (%s)
		order by rowid asc`, placeholders),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targetsByPostID := make(map[string][]string, len(postIDs))
	for _, postID := range postIDs {
		targetsByPostID[postID] = []string{}
	}
	for rows.Next() {
		var postID, accountID string
		if err := rows.Scan(&postID, &accountID); err != nil {
			return nil, err
		}
		targetsByPostID[postID] = append(targetsByPostID[postID], accountID)
	}
	return targetsByPostID, rows.Err()
}

type userScanner interface {
	Scan(dest ...any) error
}

type teamScanner interface {
	Scan(dest ...any) error
}

type membershipScanner interface {
	Scan(dest ...any) error
}

type accountScanner interface {
	Scan(dest ...any) error
}

type providerInstanceScanner interface {
	Scan(dest ...any) error
}

type postScanner interface {
	Scan(dest ...any) error
}

func scanUser(scanner userScanner) (domain.User, error) {
	var (
		user      domain.User
		isAdmin   int
		createdAt string
	)
	if err := scanner.Scan(&user.ID, &user.Email, &user.Name, &user.Subject, &isAdmin, &createdAt); err != nil {
		return domain.User{}, err
	}
	user.IsAdmin = isAdmin != 0
	var err error
	user.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func scanTeam(scanner teamScanner) (domain.Team, error) {
	var (
		team              domain.Team
		isPersonal        int
		isAIEnabled       int
		personalForUserID sql.NullString
		schedulingPrefs   sql.NullString
		brandColor        sql.NullString
		createdAt         string
	)
	if err := scanner.Scan(&team.ID, &team.Name, &team.Description, &createdAt, &isPersonal, &isAIEnabled, &personalForUserID, &schedulingPrefs, &brandColor); err != nil {
		return domain.Team{}, err
	}
	team.IsPersonal = isPersonal != 0
	team.IsAIEnabled = isAIEnabled != 0
	team.PersonalForUserID = personalForUserID.String
	team.BrandColor = brandColor.String
	if schedulingPrefs.Valid && strings.TrimSpace(schedulingPrefs.String) != "" {
		prefs, err := domain.ParseTeamSchedulingPrefsJSON(schedulingPrefs.String)
		if err != nil {
			return domain.Team{}, err
		}
		team.SchedulingPrefs = prefs
	} else {
		team.SchedulingPrefs = domain.DefaultTeamSchedulingPreferences()
	}
	var err error
	team.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Team{}, err
	}
	return team, nil
}

func scanMembership(scanner membershipScanner) (domain.TeamMembership, error) {
	var (
		membership domain.TeamMembership
		createdAt  string
	)
	if err := scanner.Scan(&membership.UserID, &membership.TeamID, &membership.Role, &createdAt); err != nil {
		return domain.TeamMembership{}, err
	}
	var err error
	membership.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.TeamMembership{}, err
	}
	return membership, nil
}

func scanAccount(scanner accountScanner) (domain.SocialAccount, error) {
	var (
		account            domain.SocialAccount
		providerInstanceID sql.NullString
		maxChars           sql.NullInt64
		accessExpiresRaw   sql.NullString
		createdAt          string
	)
	if err := scanner.Scan(
		&account.ID,
		&account.TeamID,
		&account.Name,
		&account.Provider,
		&account.AuthType,
		&providerInstanceID,
		&account.InstanceURL,
		&account.Username,
		&account.RemoteAccountID,
		&account.AvatarURL,
		&account.AccessTokenCiphertext,
		&account.RefreshTokenCiphertext,
		&maxChars,
		&accessExpiresRaw,
		&createdAt,
	); err != nil {
		return domain.SocialAccount{}, err
	}
	account.ProviderInstanceID = providerInstanceID.String
	if maxChars.Valid {
		value := int(maxChars.Int64)
		account.MaxCharsOverride = &value
	}
	if accessExpiresRaw.Valid && strings.TrimSpace(accessExpiresRaw.String) != "" {
		exp, err := parseTime(accessExpiresRaw.String)
		if err != nil {
			return domain.SocialAccount{}, err
		}
		if !exp.IsZero() {
			t := exp
			account.AccessTokenExpiresAt = &t
		}
	}
	var err error
	account.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.SocialAccount{}, err
	}
	return account, nil
}

func scanProviderInstance(scanner providerInstanceScanner) (domain.ProviderInstance, error) {
	var (
		instance   domain.ProviderInstance
		scopesJSON string
		createdAt  string
		updatedAt  string
	)
	if err := scanner.Scan(
		&instance.ID,
		&instance.Provider,
		&instance.Name,
		&instance.InstanceURL,
		&instance.ClientID,
		&instance.ClientSecretCiphertext,
		&scopesJSON,
		&instance.AuthorizationEndpoint,
		&instance.TokenEndpoint,
		&instance.CreatedByUserID,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.ProviderInstance{}, err
	}
	if err := json.Unmarshal([]byte(scopesJSON), &instance.Scopes); err != nil {
		return domain.ProviderInstance{}, fmt.Errorf("decode provider scopes: %w", err)
	}
	instance.HasClientSecret = instance.ClientSecretCiphertext != ""
	var err error
	instance.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.ProviderInstance{}, err
	}
	instance.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.ProviderInstance{}, err
	}
	return instance, nil
}

func scanPost(scanner postScanner) (domain.ScheduledPost, error) {
	var (
		post             domain.ScheduledPost
		lastError        sql.NullString
		scheduled        string
		mediaIDsJSON     string
		mediaExcludeJSON string
		postTemplateID   sql.NullString
		templateCtr      sql.NullInt64
		createdAt        string
		updatedAt        string
	)
	if err := scanner.Scan(
		&post.ID,
		&post.TeamID,
		&post.AuthorUserID,
		&post.Title,
		&post.Content,
		&scheduled,
		&post.Status,
		&post.Source,
		&post.AttemptCount,
		&lastError,
		&post.Visibility,
		&mediaIDsJSON,
		&mediaExcludeJSON,
		&postTemplateID,
		&templateCtr,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.ScheduledPost{}, err
	}
	post.LastError = lastError.String
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
	if strings.TrimSpace(mediaIDsJSON) != "" {
		if err := json.Unmarshal([]byte(mediaIDsJSON), &post.MediaIDs); err != nil {
			return domain.ScheduledPost{}, fmt.Errorf("decode media_ids: %w", err)
		}
	}
	if strings.TrimSpace(mediaExcludeJSON) != "" && mediaExcludeJSON != "{}" {
		if err := json.Unmarshal([]byte(mediaExcludeJSON), &post.MediaExcludeByAccount); err != nil {
			return domain.ScheduledPost{}, fmt.Errorf("decode media_exclude_by_account: %w", err)
		}
	}
	var err error
	post.ScheduledAt, err = parseTime(scheduled)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	post.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	post.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	return post, nil
}

func collectUsers(rows *sql.Rows) ([]domain.User, error) {
	items := make([]domain.User, 0, 8)
	for rows.Next() {
		item, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func collectTeams(rows *sql.Rows) ([]domain.Team, error) {
	items := make([]domain.Team, 0, 4)
	for rows.Next() {
		item, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func collectMemberships(rows *sql.Rows) ([]domain.TeamMembership, error) {
	items := make([]domain.TeamMembership, 0, 4)
	for rows.Next() {
		item, err := scanMembership(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func collectAccounts(rows *sql.Rows) ([]domain.SocialAccount, error) {
	items := make([]domain.SocialAccount, 0, 4)
	for rows.Next() {
		item, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func collectProviderInstances(rows *sql.Rows) ([]domain.ProviderInstance, error) {
	items := make([]domain.ProviderInstance, 0, 4)
	for rows.Next() {
		item, err := scanProviderInstance(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func collectPosts(rows *sql.Rows) ([]domain.ScheduledPost, error) {
	items := make([]domain.ScheduledPost, 0, 16)
	for rows.Next() {
		item, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func queryUser(ctx context.Context, q queryRower, query string, args ...any) (domain.User, error) {
	return scanUser(q.QueryRowContext(ctx, query, args...))
}

func queryTeam(ctx context.Context, q queryRower, query string, args ...any) (domain.Team, error) {
	return scanTeam(q.QueryRowContext(ctx, query, args...))
}

func queryMembership(ctx context.Context, q queryRower, query string, args ...any) (domain.TeamMembership, error) {
	return scanMembership(q.QueryRowContext(ctx, query, args...))
}

func queryAccount(ctx context.Context, q queryRower, query string, args ...any) (domain.SocialAccount, error) {
	return scanAccount(q.QueryRowContext(ctx, query, args...))
}

func queryProviderInstance(ctx context.Context, q queryRower, query string, args ...any) (domain.ProviderInstance, error) {
	return scanProviderInstance(q.QueryRowContext(ctx, query, args...))
}

func queryPost(ctx context.Context, q queryRower, query string, args ...any) (domain.ScheduledPost, error) {
	return scanPost(q.QueryRowContext(ctx, query, args...))
}

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func inClause(values []string) (string, []any) {
	parts := make([]string, len(values))
	args := make([]any, len(values))
	for i, value := range values {
		parts[i] = "?"
		args[i] = value
	}
	return strings.Join(parts, ", "), args
}

func encodeMediaIDsJSON(ids []string) (string, error) {
	ids = domain.NormalizeMediaIDs(ids)
	b, err := json.Marshal(ids)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func encodeMediaExcludeJSON(m map[string][]string) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func nowString() string {
	return formatTime(time.Now().UTC())
}

func formatTime(t time.Time) string {
	return t.UTC().Format(sqliteTimeLayout)
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{sqliteTimeLayout, time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("parse sqlite time %q", value)
}

func mustParseTime(value string) time.Time {
	parsed, err := parseTime(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func marshalScopes(scopes []string) (string, error) {
	if len(scopes) == 0 {
		return "[]", nil
	}
	payload, err := json.Marshal(scopes)
	if err != nil {
		return "", fmt.Errorf("encode provider scopes: %w", err)
	}
	return string(payload), nil
}
