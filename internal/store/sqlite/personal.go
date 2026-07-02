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
		`alter table teams add column is_ai_enabled integer not null default 0`,
		`alter table teams add column brand_color text not null default ''`,
		`alter table social_accounts add column avatar_url text not null default ''`,
		`alter table social_accounts add column access_token_expires_at text`,
		`alter table scheduled_posts add column visibility text not null default 'public'`,
		`alter table scheduled_posts add column media_ids text not null default '[]'`,
		`alter table scheduled_posts add column media_exclude_by_account text not null default '{}'`,
		`alter table scheduled_post_targets add column publish_metadata text not null default '{}'`,
		`alter table scheduled_post_targets add column metrics_last_sync_date text`,
		`alter table scheduled_post_targets add column metrics_last_sync_at text`,
		`alter table post_templates add column announces_template_id text references post_templates(id) on delete set null`,
		`alter table post_templates add column announcement_days_before integer`,
		`alter table post_templates add column announcement_enabled integer not null default 0`,
		`alter table post_templates add column announcement_title text not null default ''`,
		`alter table post_templates add column announcement_content text not null default ''`,
		`alter table post_templates add column announcement_counter_next integer not null default 1`,
		`alter table post_templates add column announcement_target_account_ids text not null default '[]'`,
		`alter table post_templates add column ai_enhance_enabled integer not null default 0`,
		`alter table post_templates add column ai_enhance_announcement integer not null default 0`,
		`alter table post_templates add column materialize_horizon_days integer not null default 0`,
		`alter table scheduled_posts add column template_occurrence_at text`,
		`alter table scheduled_posts add column template_post_role text not null default ''`,
		`alter table post_template_skips add column skip_scope text not null default 'occurrence'`,
		`alter table post_templates add column output_mode text not null default 'scheduled'`,
		`alter table post_templates add column prompt_hint text not null default ''`,
		`alter table post_templates add column tonality text not null default ''`,
		`alter table post_templates add column title_hint text not null default ''`,
		`alter table api_tokens add column scopes text not null default ''`,
		`alter table api_tokens add column team_id text references teams(id) on delete cascade`,
		`alter table api_tokens add column description text not null default ''`,
		`alter table scheduled_posts add column source text not null default 'scheduled'`,
		`alter table scheduled_posts add column acknowledged_at text`,
		`alter table scheduled_post_targets add column remote_post_id text`,
		`alter table rss_feed_configs add column prompt_hint text not null default ''`,
		`alter table rss_feed_configs add column target_account_ids text not null default '[]'`,
		`alter table rss_feed_configs add column tonality text not null default ''`,
		`alter table rss_feed_configs add column initial_sync_mode text not null default 'baseline'`,
		`alter table rss_feed_configs add column content_template text not null default '{title}

{link}'`,
		`alter table rss_feed_configs add column output_mode text not null default 'draft'`,
		`alter table rss_feed_configs add column max_posts_per_day integer not null default 10`,
		`alter table rss_feed_configs add column counter_next integer not null default 1`,
		`alter table rss_feed_configs add column ai_enhance_enabled integer not null default 0`,
		`alter table rss_feed_configs add column title_template text not null default '{title}'`,
		`alter table rss_feed_configs add column title_hint text not null default ''`,
		`alter table scheduled_posts add column rss_feed_id text references rss_feed_configs(id) on delete set null`,
		`create table if not exists rss_imported_items (
			id text primary key,
			feed_id text not null references rss_feed_configs(id) on delete cascade,
			item_key text not null,
			post_id text references scheduled_posts(id) on delete set null,
			created_at text not null default (datetime('now')),
			unique (feed_id, item_key)
		)`,
		`create index if not exists idx_rss_imported_items_feed on rss_imported_items(feed_id)`,
		`alter table ai_service_configs add column provider text not null default 'openai'`,
		`alter table ai_service_configs add column model text not null default ''`,
		`alter table ai_service_configs add column base_url text not null default ''`,
		`alter table ai_service_configs add column api_key_ciphertext text not null default ''`,
		`alter table ai_service_configs drop column service_url`,
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
	if _, err := db.ExecContext(ctx, `create index if not exists idx_post_targets_metrics_sync on scheduled_post_targets(metrics_last_sync_at)`); err != nil {
		return fmt.Errorf("sqlite migrate metrics sync index: %w", err)
	}
	if _, err := db.ExecContext(ctx, `create unique index if not exists ux_post_targets_account_remote_post on scheduled_post_targets(account_id, remote_post_id) where remote_post_id is not null and trim(remote_post_id) <> ''`); err != nil {
		return fmt.Errorf("sqlite migrate remote post id index: %w", err)
	}
	for _, stmt := range []string{
		`create table if not exists external_post_monitor_settings (
			id text primary key,
			team_id text not null references teams(id) on delete cascade unique,
			enabled integer not null default 0,
			backfill_completed_at text,
			last_sync_at text,
			created_at text not null,
			updated_at text not null
		)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite migrate external post monitor: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		update scheduled_post_targets
		set remote_post_id = trim(json_extract(publish_metadata, '$.uri'))
		where status = 'posted'
		  and (remote_post_id is null or trim(remote_post_id) = '')
		  and publish_metadata is not null
		  and trim(publish_metadata) <> '{}'
		  and trim(json_extract(publish_metadata, '$.uri')) <> ''`); err != nil {
		return fmt.Errorf("sqlite migrate remote post id backfill: %w", err)
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
	if err := migrateSQLiteScheduledPostsAutomationSource(ctx, db); err != nil {
		return err
	}
	if err := migrateSQLiteEmbeddedAnnouncements(ctx, db); err != nil {
		return err
	}
	if err := migrateSQLiteProviderConstraint(ctx, db); err != nil {
		return err
	}
	return nil
}

// migrateSQLiteProviderConstraint relaxes the hard-coded provider enum CHECK on
// provider_instances and social_accounts to `provider <> ''`, so new Mastodon-compatible
// providers (e.g. pixelfed) are accepted. The provider registry validates names at runtime.
func migrateSQLiteProviderConstraint(ctx context.Context, db *sql.DB) error {
	if err := rebuildSQLiteProviderTable(ctx, db, "provider_instances", `
CREATE TABLE provider_instances_new (
    id text primary key,
    provider text not null check (provider <> ''),
    name text not null,
    instance_url text not null,
    client_id text not null default '',
    client_secret_ciphertext text not null default '',
    scopes_json text not null default '[]',
    authorization_endpoint text not null default '',
    token_endpoint text not null default '',
    created_by_user_id text not null references users(id) on delete restrict,
    created_at text not null,
    updated_at text not null,
    unique (provider, instance_url)
);
INSERT INTO provider_instances_new SELECT
    id, provider, name, instance_url, client_id, client_secret_ciphertext, scopes_json,
    authorization_endpoint, token_endpoint, created_by_user_id, created_at, updated_at
FROM provider_instances;
DROP TABLE provider_instances;
ALTER TABLE provider_instances_new RENAME TO provider_instances;
CREATE INDEX IF NOT EXISTS idx_provider_instances_provider ON provider_instances(provider, instance_url);
`); err != nil {
		return err
	}

	return rebuildSQLiteProviderTable(ctx, db, "social_accounts", `
CREATE TABLE social_accounts_new (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    name text not null default '',
    provider text not null check (provider <> ''),
    auth_type text not null default 'oauth_token' check (auth_type in ('oauth_token', 'app_password')),
    provider_instance_id text references provider_instances(id) on delete set null,
    instance_url text not null,
    username text not null,
    remote_account_id text not null default '',
    avatar_url text not null default '',
    access_token_ciphertext text not null,
    refresh_token_ciphertext text not null default '',
    max_chars_override integer,
    access_token_expires_at text,
    created_at text not null
);
INSERT INTO social_accounts_new SELECT
    id, team_id, name, provider, auth_type, provider_instance_id, instance_url, username,
    remote_account_id, avatar_url, access_token_ciphertext, refresh_token_ciphertext,
    max_chars_override, access_token_expires_at, created_at
FROM social_accounts;
DROP TABLE social_accounts;
ALTER TABLE social_accounts_new RENAME TO social_accounts;
CREATE INDEX IF NOT EXISTS idx_social_accounts_team ON social_accounts(team_id);
`)
}

// rebuildSQLiteProviderTable runs a 12-step table rebuild when the existing table still
// carries the old provider enum CHECK (i.e. its create SQL lacks the relaxed marker).
func rebuildSQLiteProviderTable(ctx context.Context, db *sql.DB, table, rebuildBody string) error {
	var createSQL sql.NullString
	err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&createSQL)
	if err != nil || !createSQL.Valid || createSQL.String == "" {
		return nil
	}
	if strings.Contains(createSQL.String, "provider <> ''") {
		return nil
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=OFF;\nBEGIN TRANSACTION;\n"+rebuildBody+"COMMIT;\nPRAGMA foreign_keys=ON;\n"); err != nil {
		return fmt.Errorf("sqlite migrate %s provider constraint: %w", table, err)
	}
	return nil
}

func migrateSQLiteEmbeddedAnnouncements(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		select id, announces_template_id, title, content, announcement_days_before, counter_next, target_account_ids
		from post_templates
		where announces_template_id is not null and trim(announces_template_id) <> ''`)
	if err != nil {
		return fmt.Errorf("sqlite migrate embedded announcements: %w", err)
	}
	defer rows.Close()
	type childRow struct {
		id, parentID, title, content, targets string
		daysBefore, counterNext                int
	}
	var children []childRow
	for rows.Next() {
		var c childRow
		var days sql.NullInt64
		if err := rows.Scan(&c.id, &c.parentID, &c.title, &c.content, &days, &c.counterNext, &c.targets); err != nil {
			return err
		}
		if days.Valid {
			c.daysBefore = int(days.Int64)
		} else {
			c.daysBefore = 2
		}
		if c.counterNext < 1 {
			c.counterNext = 1
		}
		children = append(children, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range children {
		res, err := db.ExecContext(ctx, `
			update post_templates
			set announcement_enabled = 1,
			    announcement_title = ?,
			    announcement_content = ?,
			    announcement_days_before = ?,
			    announcement_counter_next = ?,
			    announcement_target_account_ids = ?,
			    updated_at = ?
			where id = ?`,
			c.title, c.content, c.daysBefore, c.counterNext, c.targets, nowString(), c.parentID,
		)
		if err != nil {
			return fmt.Errorf("sqlite migrate embedded announcements parent %s: %w", c.parentID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n > 0 {
			if _, err := db.ExecContext(ctx, `delete from post_templates where id = ?`, c.id); err != nil {
				return fmt.Errorf("sqlite migrate embedded announcements delete child %s: %w", c.id, err)
			}
			continue
		}
		// Orphan announcement child (missing parent): keep content as a standalone template.
		if _, err := db.ExecContext(ctx, `
			update post_templates
			set announces_template_id = null, updated_at = ?
			where id = ?`,
			nowString(), c.id,
		); err != nil {
			return fmt.Errorf("sqlite migrate embedded announcements promote orphan %s: %w", c.id, err)
		}
	}
	return nil
}

func migrateSQLiteScheduledPostsAutomationSource(ctx context.Context, db *sql.DB) error {
	var createSQL sql.NullString
	err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='scheduled_posts'`).Scan(&createSQL)
	if err != nil || !createSQL.Valid || createSQL.String == "" {
		return nil
	}
	if strings.Contains(createSQL.String, "'automation'") {
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
    source text not null default 'scheduled' check (source in ('scheduled', 'imported', 'automation')),
    attempt_count integer not null default 0,
    last_error text,
    visibility text not null default 'public',
    media_ids text not null default '[]',
    media_exclude_by_account text not null default '{}',
    post_template_id text,
    template_counter integer,
    rss_feed_id text references rss_feed_configs(id) on delete set null,
    created_at text not null,
    updated_at text not null
);
INSERT INTO scheduled_posts_new (
    id, team_id, author_user_id, title, content, scheduled_at, status, source,
    attempt_count, last_error, visibility, media_ids, media_exclude_by_account,
    post_template_id, template_counter, rss_feed_id, created_at, updated_at
)
SELECT id, team_id, author_user_id, title, content, scheduled_at, status,
    coalesce(nullif(trim(source), ''), 'scheduled'),
    attempt_count, last_error, visibility, media_ids,
    coalesce(nullif(trim(media_exclude_by_account), ''), '{}'),
    post_template_id, template_counter, rss_feed_id, created_at, updated_at
FROM scheduled_posts;
DROP TABLE scheduled_posts;
ALTER TABLE scheduled_posts_new RENAME TO scheduled_posts;
CREATE INDEX IF NOT EXISTS idx_scheduled_posts_due ON scheduled_posts(status, scheduled_at);
COMMIT;
PRAGMA foreign_keys=ON;
`); err != nil {
		return fmt.Errorf("sqlite migrate scheduled_posts automation source: %w", err)
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
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists") || strings.Contains(msg, "no such column")
}

// MigratePersonalWorkspaces converts legacy auto-created personal workspaces
// into regular teams: the personal markers are cleared and teams still named
// with the generated "Personal · <uid8>" pattern are renamed after their owner
// (display name, falling back to the email local part). Idempotent; runs at
// startup.
func (s *Store) MigratePersonalWorkspaces(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		select t.id, t.name, coalesce(t.personal_for_user_id, ''), coalesce(u.name, ''), coalesce(u.email, '')
		from teams t
		left join users u on u.id = t.personal_for_user_id
		where t.is_personal = 1 or t.personal_for_user_id is not null`)
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
		if _, err := s.db.ExecContext(ctx,
			`update teams set is_personal = 0, personal_for_user_id = null, name = ? where id = ?`,
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
		if err := s.db.QueryRowContext(ctx,
			`select count(*) from teams where name = ? and id <> ?`, candidate, teamID,
		).Scan(&count); err != nil || count == 0 {
			return candidate
		}
		candidate = fmt.Sprintf("%s %d", preferred, n)
	}
	return preferred + " " + teamID[:8]
}

func (s *Store) GetTeamByID(ctx context.Context, teamID string) (domain.Team, error) {
	return queryTeam(ctx, s.db, `select id, name, description, created_at, is_ai_enabled, scheduling_prefs, brand_color from teams where id = ?`, teamID)
}

func (s *Store) GetAccountByID(ctx context.Context, accountID string) (domain.SocialAccount, error) {
	return queryAccount(ctx, s.db, `
		select id, team_id, name, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
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
		select id, team_id, name, provider, auth_type, provider_instance_id, instance_url, username, remote_account_id,
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
		select id, team_id, author_user_id, title, content, scheduled_at, status, source,
		       attempt_count, last_error, visibility, media_ids, media_exclude_by_account,
		       post_template_id, template_counter, created_at, updated_at
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
	now := nowString()
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	expiresStr := formatTime(expires)

	if _, err := s.db.ExecContext(ctx, `
		insert into team_invitations (id, team_id, email, role, token_hash, expires_at, created_by_user_id, created_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, teamID, email, string(input.Role), hash, expiresStr, createdByUserID, now,
	); err != nil {
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
