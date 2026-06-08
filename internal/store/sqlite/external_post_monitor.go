package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postdedup"
	"git.f4mily.net/goloom/internal/provider"
	"github.com/google/uuid"
)

func (s *Store) GetExternalPostMonitorSettings(ctx context.Context, teamID string) (domain.ExternalPostMonitorSettings, error) {
	row, err := scanExternalPostMonitorSettings(s.db.QueryRowContext(ctx, `
		select id, team_id, enabled, backfill_completed_at, last_sync_at, created_at, updated_at
		from external_post_monitor_settings
		where team_id = ?`, teamID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ExternalPostMonitorSettings{TeamID: teamID, Enabled: false}, nil
		}
		return domain.ExternalPostMonitorSettings{}, err
	}
	return row, nil
}

func (s *Store) UpsertExternalPostMonitorSettings(ctx context.Context, teamID string, input domain.UpsertExternalPostMonitorInput) (domain.ExternalPostMonitorSettings, error) {
	existing, err := s.GetExternalPostMonitorSettings(ctx, teamID)
	if err != nil {
		return domain.ExternalPostMonitorSettings{}, err
	}
	now := nowString()
	enabledInt := 0
	if input.Enabled {
		enabledInt = 1
	}
	if existing.ID == "" {
		id := uuid.NewString()
		_, err := s.db.ExecContext(ctx, `
			insert into external_post_monitor_settings (id, team_id, enabled, backfill_completed_at, last_sync_at, created_at, updated_at)
			values (?, ?, ?, null, null, ?, ?)`,
			id, teamID, enabledInt, now, now,
		)
		if err != nil {
			return domain.ExternalPostMonitorSettings{}, err
		}
		return s.GetExternalPostMonitorSettings(ctx, teamID)
	}
	backfill := existing.BackfillCompletedAt
	if input.Enabled && !existing.Enabled {
		backfill = nil
	}
	var backfillStr any
	if backfill != nil {
		backfillStr = formatTime(*backfill)
	}
	var lastSyncStr any
	if existing.LastSyncAt != nil {
		lastSyncStr = formatTime(*existing.LastSyncAt)
	}
	_, err = s.db.ExecContext(ctx, `
		update external_post_monitor_settings
		set enabled = ?, backfill_completed_at = ?, last_sync_at = ?, updated_at = ?
		where team_id = ?`,
		enabledInt, backfillStr, lastSyncStr, now, teamID,
	)
	if err != nil {
		return domain.ExternalPostMonitorSettings{}, err
	}
	return s.GetExternalPostMonitorSettings(ctx, teamID)
}

func (s *Store) ListTeamsWithExternalPostMonitorEnabled(ctx context.Context, limit int) ([]domain.ExternalPostMonitorSettings, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, enabled, backfill_completed_at, last_sync_at, created_at, updated_at
		from external_post_monitor_settings
		where enabled = 1
		order by coalesce(last_sync_at, '') asc
		limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ExternalPostMonitorSettings
	for rows.Next() {
		row, err := scanExternalPostMonitorSettings(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UpdateExternalPostMonitorSyncState(ctx context.Context, teamID string, lastSyncAt time.Time, backfillCompleted bool) error {
	existing, err := s.GetExternalPostMonitorSettings(ctx, teamID)
	if err != nil {
		return err
	}
	if existing.ID == "" {
		return nil
	}
	syncStr := formatTime(lastSyncAt.UTC())
	var backfillStr any
	if backfillCompleted {
		if existing.BackfillCompletedAt != nil {
			backfillStr = formatTime(*existing.BackfillCompletedAt)
		} else {
			backfillStr = syncStr
		}
	} else if existing.BackfillCompletedAt != nil {
		backfillStr = formatTime(*existing.BackfillCompletedAt)
	}
	_, err = s.db.ExecContext(ctx, `
		update external_post_monitor_settings
		set last_sync_at = ?, backfill_completed_at = ?, updated_at = ?
		where team_id = ?`,
		syncStr, backfillStr, nowString(), teamID,
	)
	return err
}

func (s *Store) TargetExistsByRemotePostID(ctx context.Context, accountID, remotePostID string) (bool, error) {
	return s.AuthorPostAlreadyTracked(ctx, accountID, remotePostID, "", nil)
}

func (s *Store) AuthorPostAlreadyTracked(ctx context.Context, accountID, remoteID, publishedURL string, metadata map[string]string) (bool, error) {
	candidateSet := make(map[string]struct{})
	for _, id := range provider.CollectAuthorPostIdentifiers(remoteID, publishedURL, metadata) {
		candidateSet[id] = struct{}{}
	}
	if len(candidateSet) == 0 {
		return false, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		select coalesce(remote_post_id, ''), coalesce(published_url, ''), coalesce(publish_metadata, '{}')
		from scheduled_post_targets
		where account_id = ? and status = 'posted'`, accountID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var remote, url, metaRaw string
		if err := rows.Scan(&remote, &url, &metaRaw); err != nil {
			return false, err
		}
		for _, id := range provider.CollectAuthorPostIdentifiers(remote, url, postdedup.ParsePublishMetadata(metaRaw)) {
			if _, ok := candidateSet[id]; ok {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}

func (s *Store) DeleteRedundantImportedPosts(ctx context.Context, teamID string) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		select p.id, p.source, t.account_id,
		       coalesce(t.remote_post_id, ''), coalesce(t.published_url, ''), coalesce(t.publish_metadata, '{}')
		from scheduled_posts p
		inner join scheduled_post_targets t on t.post_id = p.id
		where p.team_id = ? and p.status = 'posted' and t.status = 'posted'`,
		teamID,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var refs []postdedup.PostedTargetRef
	for rows.Next() {
		var ref postdedup.PostedTargetRef
		var source string
		var metaRaw string
		if err := rows.Scan(&ref.PostID, &source, &ref.AccountID, &ref.RemotePostID, &ref.PublishedURL, &metaRaw); err != nil {
			return 0, err
		}
		ref.PostSource = domain.PostSource(source)
		ref.PublishMetadata = postdedup.ParsePublishMetadata(metaRaw)
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	deleted := 0
	for _, postID := range postdedup.RedundantImportedPostIDs(refs) {
		if err := s.DeleteScheduledPost(ctx, teamID, postID); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func (s *Store) CreateImportedPost(ctx context.Context, teamID, authorUserID string, input domain.ImportedPostInput) (domain.ScheduledPost, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	defer tx.Rollback()

	postID := uuid.NewString()
	now := nowString()
	publishedAt := formatTime(input.PublishedAt.UTC())
	metaJSON := "{}"
	if input.PublishMetadata != nil {
		b, err := json.Marshal(input.PublishMetadata)
		if err != nil {
			return domain.ScheduledPost{}, err
		}
		metaJSON = string(b)
	}

	if _, err := tx.ExecContext(ctx, `
		insert into scheduled_posts (
			id, team_id, author_user_id, title, content, scheduled_at, status, source,
			attempt_count, visibility, media_ids, media_exclude_by_account, created_at, updated_at
		) values (?, ?, ?, '', ?, ?, ?, ?, 0, 'public', '[]', '{}', ?, ?)`,
		postID, teamID, authorUserID, input.Content, publishedAt,
		domain.PostStatusPosted, domain.PostSourceImported, now, now,
	); err != nil {
		return domain.ScheduledPost{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		insert into scheduled_post_targets (post_id, account_id, status, published_url, publish_metadata, remote_post_id)
		values (?, ?, ?, ?, ?, ?)`,
		postID, input.AccountID, domain.PostStatusPosted,
		strings.TrimSpace(input.PublishedURL), metaJSON, strings.TrimSpace(input.RemotePostID),
	); err != nil {
		return domain.ScheduledPost{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.ScheduledPost{}, err
	}
	return s.GetScheduledPost(ctx, teamID, postID)
}

func scanExternalPostMonitorSettings(scanner interface{ Scan(dest ...any) error }) (domain.ExternalPostMonitorSettings, error) {
	var out domain.ExternalPostMonitorSettings
	var enabledInt int
	var backfillRaw, lastSyncRaw, createdRaw, updatedRaw sql.NullString
	if err := scanner.Scan(
		&out.ID,
		&out.TeamID,
		&enabledInt,
		&backfillRaw,
		&lastSyncRaw,
		&createdRaw,
		&updatedRaw,
	); err != nil {
		return domain.ExternalPostMonitorSettings{}, err
	}
	out.Enabled = enabledInt != 0
	if backfillRaw.Valid && strings.TrimSpace(backfillRaw.String) != "" {
		t, err := parseTime(backfillRaw.String)
		if err == nil {
			out.BackfillCompletedAt = &t
		}
	}
	if lastSyncRaw.Valid && strings.TrimSpace(lastSyncRaw.String) != "" {
		t, err := parseTime(lastSyncRaw.String)
		if err == nil {
			out.LastSyncAt = &t
		}
	}
	if createdRaw.Valid {
		out.CreatedAt, _ = parseTime(createdRaw.String)
	}
	if updatedRaw.Valid {
		out.UpdatedAt, _ = parseTime(updatedRaw.String)
	}
	return out, nil
}
