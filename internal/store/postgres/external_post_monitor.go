package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) GetExternalPostMonitorSettings(ctx context.Context, teamID string) (domain.ExternalPostMonitorSettings, error) {
	const query = `
		select id, team_id, enabled, backfill_completed_at, last_sync_at, created_at, updated_at
		from external_post_monitor_settings
		where team_id = $1
	`
	row, err := scanExternalPostMonitorSettings(s.pool.QueryRow(ctx, query, teamID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ExternalPostMonitorSettings{TeamID: teamID, Enabled: false}, nil
		}
		return domain.ExternalPostMonitorSettings{}, fmt.Errorf("GetExternalPostMonitorSettings: %w", err)
	}
	return row, nil
}

func (s *Store) UpsertExternalPostMonitorSettings(ctx context.Context, teamID string, input domain.UpsertExternalPostMonitorInput) (domain.ExternalPostMonitorSettings, error) {
	const query = `
		insert into external_post_monitor_settings (team_id, enabled)
		values ($1, $2)
		on conflict (team_id) do update
		set enabled = excluded.enabled,
		    backfill_completed_at = case
		        when excluded.enabled = true and external_post_monitor_settings.enabled = false then null
		        else external_post_monitor_settings.backfill_completed_at
		    end,
		    updated_at = now()
		returning id, team_id, enabled, backfill_completed_at, last_sync_at, created_at, updated_at
	`
	row, err := scanExternalPostMonitorSettings(s.pool.QueryRow(ctx, query, teamID, input.Enabled))
	if err != nil {
		return domain.ExternalPostMonitorSettings{}, fmt.Errorf("UpsertExternalPostMonitorSettings: %w", err)
	}
	return row, nil
}

func (s *Store) ListTeamsWithExternalPostMonitorEnabled(ctx context.Context, limit int) ([]domain.ExternalPostMonitorSettings, error) {
	if limit <= 0 {
		limit = 200
	}
	const query = `
		select id, team_id, enabled, backfill_completed_at, last_sync_at, created_at, updated_at
		from external_post_monitor_settings
		where enabled = true
		order by coalesce(last_sync_at, '1970-01-01'::timestamptz) asc
		limit $1
	`
	rows, err := s.pool.Query(ctx, query, limit)
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
	_, err := s.pool.Exec(ctx, `
		update external_post_monitor_settings
		set last_sync_at = $1,
		    backfill_completed_at = case when $2 then coalesce(backfill_completed_at, $1) else backfill_completed_at end,
		    updated_at = now()
		where team_id = $3`,
		lastSyncAt.UTC(), backfillCompleted, teamID,
	)
	return err
}

func (s *Store) TargetExistsByRemotePostID(ctx context.Context, accountID, remotePostID string) (bool, error) {
	remotePostID = strings.TrimSpace(remotePostID)
	if remotePostID == "" {
		return false, nil
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `
		select exists(
			select 1 from scheduled_post_targets
			where account_id = $1 and remote_post_id = $2
		)`, accountID, remotePostID).Scan(&exists)
	return exists, err
}

func (s *Store) CreateImportedPost(ctx context.Context, teamID, authorUserID string, input domain.ImportedPostInput) (domain.ScheduledPost, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	defer tx.Rollback(ctx)

	postID := uuid.NewString()
	publishedAt := input.PublishedAt.UTC()
	metaJSON := "{}"
	if input.PublishMetadata != nil {
		b, err := json.Marshal(input.PublishMetadata)
		if err != nil {
			return domain.ScheduledPost{}, err
		}
		metaJSON = string(b)
	}

	const insertPost = `
		insert into scheduled_posts (
			id, team_id, author_user_id, title, content, scheduled_at, status, source, visibility, media_ids, media_exclude_by_account
		) values ($1, $2, $3, '', $4, $5, $6, $7, 'public', '[]', '{}')
	`
	if _, err := tx.Exec(ctx, insertPost,
		postID, teamID, authorUserID, input.Content, publishedAt,
		domain.PostStatusPosted, domain.PostSourceImported,
	); err != nil {
		return domain.ScheduledPost{}, err
	}

	if _, err := tx.Exec(ctx, `
		insert into scheduled_post_targets (post_id, account_id, status, published_url, publish_metadata, remote_post_id)
		values ($1, $2, $3, $4, $5, $6)`,
		postID, input.AccountID, domain.PostStatusPosted,
		strings.TrimSpace(input.PublishedURL), metaJSON, strings.TrimSpace(input.RemotePostID),
	); err != nil {
		return domain.ScheduledPost{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.ScheduledPost{}, err
	}
	return s.GetScheduledPost(ctx, teamID, postID)
}

func scanExternalPostMonitorSettings(row interface{ Scan(dest ...any) error }) (domain.ExternalPostMonitorSettings, error) {
	var out domain.ExternalPostMonitorSettings
	var backfill, lastSync *time.Time
	if err := row.Scan(
		&out.ID,
		&out.TeamID,
		&out.Enabled,
		&backfill,
		&lastSync,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return domain.ExternalPostMonitorSettings{}, err
	}
	out.BackfillCompletedAt = backfill
	out.LastSyncAt = lastSync
	return out, nil
}
