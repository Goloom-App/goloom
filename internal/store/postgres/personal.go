package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MigratePersonalWorkspaces converts legacy auto-created personal workspaces
// into regular teams: the personal markers are cleared and teams still named
// with the generated "Personal · <uid8>" pattern are renamed after their owner
// (display name, falling back to the email local part). Idempotent; runs at
// startup.
func (s *Store) MigratePersonalWorkspaces(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		select t.id, t.name, coalesce(t.personal_for_user_id::text, ''), coalesce(u.name, ''), coalesce(u.email, '')
		from teams t
		left join users u on u.id = t.personal_for_user_id
		where t.is_personal or t.personal_for_user_id is not null`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type legacyTeam struct {
		id, name, userID, userName, userEmail string
	}
	var teams []legacyTeam
	for rows.Next() {
		var lt legacyTeam
		if err := rows.Scan(&lt.id, &lt.name, &lt.userID, &lt.userName, &lt.userEmail); err != nil {
			return err
		}
		teams = append(teams, lt)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, lt := range teams {
		name := lt.name
		if lt.userID != "" && lt.name == legacyPersonalTeamName(lt.userID) {
			name = s.uniqueTeamName(ctx, lt.id, preferredTeamName(lt.userName, lt.userEmail, lt.name))
		}
		if _, err := s.pool.Exec(ctx,
			`update teams set is_personal = false, personal_for_user_id = null, name = $1 where id = $2`,
			name, lt.id,
		); err != nil {
			return err
		}
	}
	return nil
}

func legacyPersonalTeamName(userID string) string {
	if len(userID) < 8 {
		return "Personal · " + userID
	}
	return "Personal · " + userID[:8]
}

// preferredTeamName picks the rename target for a migrated personal workspace.
func preferredTeamName(userName, userEmail, fallback string) string {
	if name := strings.TrimSpace(userName); name != "" {
		return name
	}
	if at := strings.IndexByte(userEmail, '@'); at > 0 {
		return userEmail[:at]
	}
	return fallback
}

// uniqueTeamName returns preferred, or "<preferred> 2", "<preferred> 3", …
// when the name is already taken by another team (teams.name is unique).
func (s *Store) uniqueTeamName(ctx context.Context, teamID, preferred string) string {
	candidate := preferred
	for n := 2; n < 100; n++ {
		var count int
		if err := s.pool.QueryRow(ctx,
			`select count(*) from teams where name = $1 and id <> $2`, candidate, teamID,
		).Scan(&count); err != nil || count == 0 {
			return candidate
		}
		candidate = fmt.Sprintf("%s %d", preferred, n)
	}
	return preferred + " " + teamID[:8]
}

func (s *Store) GetTeamByID(ctx context.Context, teamID string) (domain.Team, error) {
	return scanTeamRow(s.pool.QueryRow(ctx, `
		select id, name, description, created_at, is_ai_enabled, scheduling_prefs, brand_color
		from teams where id = $1`,
		teamID))
}

func (s *Store) GetAccountByID(ctx context.Context, accountID string) (domain.SocialAccount, error) {
	const q = `
		select id, team_id, name, provider, auth_type, coalesce(provider_instance_id::text, ''), instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts where id = $1`
	var account domain.SocialAccount
	var accessExpires sql.NullTime
	err := s.pool.QueryRow(ctx, q, accountID).Scan(
		&account.ID, &account.TeamID, &account.Name, &account.Provider, &account.AuthType, &account.ProviderInstanceID,
		&account.InstanceURL, &account.Username, &account.RemoteAccountID,
		&account.AvatarURL,
		&account.AccessTokenCiphertext, &account.RefreshTokenCiphertext, &account.MaxCharsOverride, &accessExpires, &account.CreatedAt,
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

func (s *Store) GetAccountsByIDsGlobal(ctx context.Context, ids []string) ([]domain.SocialAccount, error) {
	if len(ids) == 0 {
		return nil, errors.New("no target accounts")
	}
	const q = `
		select id, team_id, name, provider, auth_type, coalesce(provider_instance_id::text, ''), instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts
		where id = any($1)`
	rows, err := s.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []domain.SocialAccount
	for rows.Next() {
		var account domain.SocialAccount
		var accessExpires sql.NullTime
		if err := rows.Scan(
			&account.ID, &account.TeamID, &account.Name, &account.Provider, &account.AuthType, &account.ProviderInstanceID,
			&account.InstanceURL, &account.Username, &account.RemoteAccountID,
			&account.AvatarURL,
			&account.AccessTokenCiphertext, &account.RefreshTokenCiphertext, &account.MaxCharsOverride, &accessExpires, &account.CreatedAt,
		); err != nil {
			return nil, err
		}
		if accessExpires.Valid {
			t := accessExpires.Time.UTC()
			account.AccessTokenExpiresAt = &t
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(accounts) != len(ids) {
		return nil, errors.New("one or more target accounts are missing")
	}
	var teamID string
	for _, a := range accounts {
		if teamID == "" {
			teamID = a.TeamID
			continue
		}
		if a.TeamID != teamID {
			return nil, errors.New("target accounts must belong to the same team")
		}
	}
	return accounts, nil
}

func (s *Store) GetScheduledPostByID(ctx context.Context, postID string) (domain.ScheduledPost, error) {
	const query = `
		select p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status, p.source,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       p.visibility, p.media_ids, coalesce(p.media_exclude_by_account::text, '{}'),
		       p.post_template_id::text, p.template_counter,
		       coalesce(array_agg(t.account_id::text) filter (where t.account_id is not null), '{}')
		from scheduled_posts p
		left join scheduled_post_targets t on t.post_id = p.id
		where p.id = $1
		group by p.id`
	posts, err := s.listPosts(ctx, query, postID)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	if len(posts) == 0 {
		return domain.ScheduledPost{}, errors.New("post not found")
	}
	return posts[0], nil
}

func (s *Store) DeleteSocialAccount(ctx context.Context, accountID string) error {
	_, err := s.pool.Exec(ctx, `delete from social_accounts where id = $1`, accountID)
	return err
}

func (s *Store) MigrateAccountToTeam(ctx context.Context, userID string, accountID, targetTeamID string, isAdmin bool) error {
	srcAccount, err := s.GetAccountByID(ctx, accountID)
	if err != nil {
		return err
	}
	if _, err := s.GetTeamByID(ctx, targetTeamID); err != nil {
		return err
	}
	if !isAdmin {
		ok, err := s.UserHasAnyTeamRole(ctx, userID, srcAccount.TeamID, domain.RoleOwner)
		if err != nil || !ok {
			return errors.New("only team owners can move accounts out of a team (or admin)")
		}
	}
	ok, err := s.UserHasAnyTeamRole(ctx, userID, targetTeamID, domain.RoleEditor, domain.RoleOwner)
	if err != nil || !ok {
		return errors.New("forbidden")
	}

	rows, err := s.pool.Query(ctx, `
		select distinct p.id
		from scheduled_posts p
		join scheduled_post_targets t on t.post_id = p.id
		where t.account_id = $1 and p.team_id = $2`,
		accountID, srcAccount.TeamID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	var postIDs []string
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			return err
		}
		postIDs = append(postIDs, pid)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, pid := range postIDs {
		var n int
		if err := s.pool.QueryRow(ctx, `select count(*) from scheduled_post_targets where post_id = $1`, pid).Scan(&n); err != nil {
			return err
		}
		if n > 1 {
			return errors.New("cannot migrate: scheduled posts reference multiple accounts; edit or cancel them first")
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, pid := range postIDs {
		if _, err := tx.Exec(ctx, `update scheduled_posts set team_id = $1, updated_at = now() where id = $2`, targetTeamID, pid); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `update social_accounts set team_id = $1 where id = $2`, targetTeamID, accountID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CreateTeamInvitation(ctx context.Context, teamID, createdByUserID string, input domain.CreateTeamInvitationInput) (domain.TeamInvitation, string, error) {
	if _, err := s.GetTeamByID(ctx, teamID); err != nil {
		return domain.TeamInvitation{}, "", err
	}
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" {
		return domain.TeamInvitation{}, "", errors.New("email is required")
	}
	if input.Role != domain.RoleEditor && input.Role != domain.RoleViewer {
		return domain.TeamInvitation{}, "", errors.New("role must be editor or viewer")
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return domain.TeamInvitation{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := security.HashToken(token)

	id := uuid.NewString()
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)

	var inv domain.TeamInvitation
	err := s.pool.QueryRow(ctx, `
		insert into team_invitations (id, team_id, email, role, token_hash, expires_at, created_by_user_id)
		values ($1, $2, $3, $4, $5, $6, $7)
		returning id, team_id, email, role, expires_at, created_by_user_id, created_at`,
		id, teamID, email, string(input.Role), hash, expires, createdByUserID,
	).Scan(&inv.ID, &inv.TeamID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedByUserID, &inv.CreatedAt)
	if err != nil {
		return domain.TeamInvitation{}, "", err
	}
	return inv, token, nil
}

func (s *Store) AcceptTeamInvitation(ctx context.Context, userID, email, rawToken string) (domain.TeamMembership, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	hash := security.HashToken(rawToken)

	var inv struct {
		id, teamID, invEmail, role string
		expiresAt                  time.Time
	}
	err := s.pool.QueryRow(ctx, `
		select id, team_id, email, role, expires_at
		from team_invitations
		where token_hash = $1`, hash,
	).Scan(&inv.id, &inv.teamID, &inv.invEmail, &inv.role, &inv.expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TeamMembership{}, errors.New("invalid or expired invitation")
		}
		return domain.TeamMembership{}, err
	}
	if time.Now().UTC().After(inv.expiresAt) {
		return domain.TeamMembership{}, errors.New("invitation expired")
	}
	if inv.invEmail != email {
		return domain.TeamMembership{}, errors.New("invitation email does not match your account")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.TeamMembership{}, err
	}
	defer tx.Rollback(ctx)

	var membership domain.TeamMembership
	err = tx.QueryRow(ctx, `
		insert into team_memberships (user_id, team_id, role)
		values ($1, $2, $3)
		on conflict (user_id, team_id) do update set role = excluded.role
		returning user_id, team_id, role, created_at`,
		userID, inv.teamID, domain.TeamRole(inv.role),
	).Scan(&membership.UserID, &membership.TeamID, &membership.Role, &membership.CreatedAt)
	if err != nil {
		return domain.TeamMembership{}, err
	}
	if _, err := tx.Exec(ctx, `delete from team_invitations where id = $1`, inv.id); err != nil {
		return domain.TeamMembership{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.TeamMembership{}, err
	}
	return membership, nil
}
