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

func (s *Store) EnsurePersonalTeam(ctx context.Context, userID string) (domain.Team, error) {
	var existingID string
	err := s.pool.QueryRow(ctx, `select id from teams where personal_for_user_id = $1`, userID).Scan(&existingID)
	if err == nil {
		return s.GetTeamByID(ctx, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Team{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Team{}, err
	}
	defer tx.Rollback(ctx)

	teamID := uuid.NewString()
	name := fmt.Sprintf("Personal · %s", userID[:8])
	if _, err := tx.Exec(ctx, `
		insert into teams (id, name, description, is_personal, personal_for_user_id)
		values ($1, $2, '', true, $3)`,
		teamID, name, userID,
	); err != nil {
		return domain.Team{}, err
	}

	if _, err := tx.Exec(ctx, `insert into team_memberships (user_id, team_id, role) values ($1, $2, $3)`, userID, teamID, domain.RoleOwner); err != nil {
		return domain.Team{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Team{}, err
	}
	return s.GetTeamByID(ctx, teamID)
}

func (s *Store) EnsurePersonalTeamsMigrated(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `select id from users`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.EnsurePersonalTeam(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetTeamByID(ctx context.Context, teamID string) (domain.Team, error) {
	return scanTeamRow(s.pool.QueryRow(ctx, `
		select id, name, description, created_at, is_personal, personal_for_user_id, scheduling_prefs
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
		select p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status,
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
	srcTeam, err := s.GetTeamByID(ctx, srcAccount.TeamID)
	if err != nil {
		return err
	}
	dstTeam, err := s.GetTeamByID(ctx, targetTeamID)
	if err != nil {
		return err
	}
	if dstTeam.IsPersonal {
		return errors.New("cannot migrate into a personal workspace")
	}
	if !srcTeam.IsPersonal && !isAdmin {
		return errors.New("only accounts in your personal workspace can be migrated (or admin)")
	}
	if srcTeam.IsPersonal && srcTeam.PersonalForUserID != userID && !isAdmin {
		return errors.New("forbidden")
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
	team, err := s.GetTeamByID(ctx, teamID)
	if err != nil {
		return domain.TeamInvitation{}, "", err
	}
	if team.IsPersonal {
		return domain.TeamInvitation{}, "", errors.New("cannot invite users to a personal workspace")
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
	err = s.pool.QueryRow(ctx, `
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
