package sqlite

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
)

func applySQLiteLegacyMigrations(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`alter table teams add column scheduling_prefs text not null default '{}'`,
		`alter table scheduled_posts add column post_template_id text`,
		`alter table scheduled_posts add column template_counter integer`,
		`alter table teams add column is_personal integer not null default 0`,
		`alter table teams add column personal_for_user_id text references users(id) on delete cascade`,
		`alter table social_accounts add column avatar_url text not null default ''`,
		`alter table social_accounts add column access_token_expires_at text`,
		`alter table scheduled_posts add column visibility text not null default 'public'`,
		`alter table scheduled_posts add column media_ids text not null default '[]'`,
		`alter table scheduled_posts add column media_exclude_by_account text not null default '{}'`,
		`alter table scheduled_post_targets add column publish_metadata text not null default '{}'`,
		`alter table scheduled_post_targets add column metrics_last_sync_date text`,
		`alter table scheduled_post_targets add column metrics_last_sync_at text`,
	}
	for _, s := range stmts {
		_, err := db.ExecContext(ctx, s)
		if err != nil && !sqliteIgnoreDuplicateColumn(err) {
			return fmt.Errorf("sqlite migrate: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `create unique index if not exists idx_teams_personal_user on teams(personal_for_user_id) where personal_for_user_id is not null`); err != nil {
		return fmt.Errorf("sqlite migrate index: %w", err)
	}
	for _, stmt := range []string{
		`create table if not exists account_metrics (
			account_id text not null references social_accounts(id) on delete cascade,
			metric text not null,
			value integer not null default 0,
			updated_at text not null,
			primary key (account_id, metric)
		)`,
		`create table if not exists account_metrics_history (
			account_id text not null references social_accounts(id) on delete cascade,
			metric text not null,
			value integer not null default 0,
			recorded_at text not null,
			primary key (account_id, metric, recorded_at)
		)`,
		`create index if not exists idx_account_metrics_history_recorded on account_metrics_history(recorded_at)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite migrate account metrics tables: %w", err)
		}
	}
	if err := migrateSQLiteScheduledPostsDraftStatus(ctx, db); err != nil {
		return err
	}
	return nil
}

func migrateSQLiteScheduledPostsDraftStatus(ctx context.Context, db *sql.DB) error {
	var createSQL sql.NullString
	err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='scheduled_posts'`).Scan(&createSQL)
	if err != nil || !createSQL.Valid || createSQL.String == "" {
		return nil
	}
	if strings.Contains(createSQL.String, "'draft'") {
		return nil
	}
	if _, err := db.ExecContext(ctx, `
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE scheduled_posts_new (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    author_user_id text not null references users(id) on delete restrict,
    title text not null default '',
    content text not null,
    scheduled_at text not null,
    status text not null check (status in ('pending', 'processing', 'posted', 'failed', 'cancelled', 'draft')),
    attempt_count integer not null default 0,
    last_error text,
    visibility text not null default 'public',
    media_ids text not null default '[]',
    media_exclude_by_account text not null default '{}',
    created_at text not null,
    updated_at text not null
);
INSERT INTO scheduled_posts_new (
    id, team_id, author_user_id, title, content, scheduled_at, status,
    attempt_count, last_error, visibility, media_ids, media_exclude_by_account, created_at, updated_at
)
SELECT id, team_id, author_user_id, title, content, scheduled_at, status,
    attempt_count, last_error, visibility, media_ids,
    coalesce(nullif(trim(media_exclude_by_account), ''), '{}'), created_at, updated_at
FROM scheduled_posts;
DROP TABLE scheduled_posts;
ALTER TABLE scheduled_posts_new RENAME TO scheduled_posts;
CREATE INDEX IF NOT EXISTS idx_scheduled_posts_due ON scheduled_posts(status, scheduled_at);
COMMIT;
PRAGMA foreign_keys=ON;
`); err != nil {
		return fmt.Errorf("sqlite migrate scheduled_posts draft status: %w", err)
	}
	return nil
}

func sqliteIgnoreDuplicateColumn(err error) bool {
	if err == nil {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}

func (s *Store) EnsurePersonalTeam(ctx context.Context, userID string) (domain.Team, error) {
	var existingID string
	err := s.db.QueryRowContext(ctx, `select id from teams where personal_for_user_id = ?`, userID).Scan(&existingID)
	if err == nil {
		return queryTeam(ctx, s.db, `select id, name, description, created_at, is_personal, personal_for_user_id, scheduling_prefs from teams where id = ?`, existingID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.Team{}, err
	}

	u, err := queryUser(ctx, s.db, `select id, email, name, subject, is_admin, created_at from users where id = ?`, userID)
	if err != nil {
		return domain.Team{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Team{}, err
	}
	defer tx.Rollback()

	teamID := uuid.NewString()
	now := nowString()
	name := fmt.Sprintf("Personal · %s", userID[:8])
	if _, err := tx.ExecContext(ctx, `
		insert into teams (id, name, description, is_personal, personal_for_user_id, created_at)
		values (?, ?, '', 1, ?, ?)`,
		teamID, name, userID, now,
	); err != nil {
		return domain.Team{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		insert into team_memberships (user_id, team_id, role, created_at)
		values (?, ?, ?, ?)`,
		userID, teamID, domain.RoleOwner, now,
	); err != nil {
		return domain.Team{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Team{}, err
	}
	_ = u
	return queryTeam(ctx, s.db, `select id, name, description, created_at, is_personal, personal_for_user_id, scheduling_prefs from teams where id = ?`, teamID)
}

func (s *Store) EnsurePersonalTeamsMigrated(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `select id from users`)
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
	return queryTeam(ctx, s.db, `select id, name, description, created_at, is_personal, personal_for_user_id, scheduling_prefs from teams where id = ?`, teamID)
}

func (s *Store) GetAccountByID(ctx context.Context, accountID string) (domain.SocialAccount, error) {
	return queryAccount(ctx, s.db, `
		select id, team_id, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts
		where id = ?`, accountID)
}

func (s *Store) GetAccountsByIDsGlobal(ctx context.Context, ids []string) ([]domain.SocialAccount, error) {
	if len(ids) == 0 {
		return nil, errors.New("no target accounts")
	}
	placeholders, args := inClause(ids)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		select id, team_id, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
		       avatar_url,
		       access_token_ciphertext, refresh_token_ciphertext, max_chars_override, access_token_expires_at, created_at
		from social_accounts
		where id in (%s)`, placeholders),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts, err := collectAccounts(rows)
	if err != nil {
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
	post, err := queryPost(ctx, s.db, `
		select id, team_id, author_user_id, title, content, scheduled_at, status,
		       attempt_count, last_error, visibility, media_ids, media_exclude_by_account, created_at, updated_at
		from scheduled_posts
		where id = ?`, postID)
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

func (s *Store) DeleteSocialAccount(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx, `delete from social_accounts where id = ?`, accountID)
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

	rows, err := s.db.QueryContext(ctx, `
		select distinct p.id
		from scheduled_posts p
		join scheduled_post_targets t on t.post_id = p.id
		where t.account_id = ? and p.team_id = ?`,
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
		if err := s.db.QueryRowContext(ctx, `select count(*) from scheduled_post_targets where post_id = ?`, pid).Scan(&n); err != nil {
			return err
		}
		if n > 1 {
			return errors.New("cannot migrate: scheduled posts reference multiple accounts; edit or cancel them first")
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, pid := range postIDs {
		if _, err := tx.ExecContext(ctx, `update scheduled_posts set team_id = ?, updated_at = ? where id = ?`, targetTeamID, nowString(), pid); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `update social_accounts set team_id = ? where id = ?`, targetTeamID, accountID); err != nil {
		return err
	}
	return tx.Commit()
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
	now := nowString()
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	expiresStr := formatTime(expires)

	_, err = s.db.ExecContext(ctx, `
		insert into team_invitations (id, team_id, email, role, token_hash, expires_at, created_by_user_id, created_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, teamID, email, string(input.Role), hash, expiresStr, createdByUserID, now,
	)
	if err != nil {
		return domain.TeamInvitation{}, "", err
	}

	inv := domain.TeamInvitation{
		ID:              id,
		TeamID:          teamID,
		Email:           email,
		Role:            input.Role,
		ExpiresAt:       expires,
		CreatedByUserID: createdByUserID,
		CreatedAt:       mustParseTime(now),
	}
	return inv, token, nil
}

func (s *Store) AcceptTeamInvitation(ctx context.Context, userID, email, rawToken string) (domain.TeamMembership, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	hash := security.HashToken(rawToken)

	var inv struct {
		id, teamID, invEmail, role string
		expiresAt                  string
	}
	err := s.db.QueryRowContext(ctx, `
		select id, team_id, email, role, expires_at
		from team_invitations
		where token_hash = ?`, hash,
	).Scan(&inv.id, &inv.teamID, &inv.invEmail, &inv.role, &inv.expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.TeamMembership{}, errors.New("invalid or expired invitation")
		}
		return domain.TeamMembership{}, err
	}
	exp, err := parseTime(inv.expiresAt)
	if err != nil {
		return domain.TeamMembership{}, err
	}
	if time.Now().UTC().After(exp) {
		return domain.TeamMembership{}, errors.New("invitation expired")
	}

	if inv.invEmail != email {
		return domain.TeamMembership{}, errors.New("invitation email does not match your account")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TeamMembership{}, err
	}
	defer tx.Rollback()

	now := nowString()
	_, err = tx.ExecContext(ctx, `
		insert into team_memberships (user_id, team_id, role, created_at)
		values (?, ?, ?, ?)
		on conflict(user_id, team_id) do update set role = excluded.role`,
		userID, inv.teamID, domain.TeamRole(inv.role), now,
	)
	if err != nil {
		return domain.TeamMembership{}, err
	}
	if _, err := tx.ExecContext(ctx, `delete from team_invitations where id = ?`, inv.id); err != nil {
		return domain.TeamMembership{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TeamMembership{}, err
	}

	return queryMembership(ctx, s.db, `
		select user_id, team_id, role, created_at
		from team_memberships
		where user_id = ? and team_id = ?`,
		userID, inv.teamID,
	)
}
