package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) CreateMediaItem(ctx context.Context, item domain.MediaItem) (domain.MediaItem, error) {
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		insert into media_items (id, team_id, sha256, filename, mime_type, size_bytes, width, height, created_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.TeamID, item.Sha256, item.Filename, item.MimeType, item.SizeBytes, item.Width, item.Height, now)
	if err != nil {
		return domain.MediaItem{}, err
	}
	return s.GetMediaItemByID(ctx, item.TeamID, item.ID)
}

func (s *Store) GetMediaItemByID(ctx context.Context, teamID, mediaID string) (domain.MediaItem, error) {
	var item domain.MediaItem
	var createdAtStr string
	err := s.db.QueryRowContext(ctx, `
		select id, team_id, sha256, filename, mime_type, size_bytes, width, height, created_at
		from media_items
		where id = ? and team_id = ?
	`, mediaID, teamID).Scan(
		&item.ID, &item.TeamID, &item.Sha256, &item.Filename, &item.MimeType,
		&item.SizeBytes, &item.Width, &item.Height, &createdAtStr,
	)
	if err != nil {
		return domain.MediaItem{}, err
	}
	item.CreatedAt, _ = parseTime(createdAtStr)
	return item, nil
}

func (s *Store) ListTeamMedia(ctx context.Context, teamID string) ([]domain.MediaItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, sha256, filename, mime_type, size_bytes, width, height, created_at
		from media_items
		where team_id = ?
		order by created_at desc
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.MediaItem
	for rows.Next() {
		var item domain.MediaItem
		var createdAtStr string
		if err := rows.Scan(
			&item.ID, &item.TeamID, &item.Sha256, &item.Filename, &item.MimeType,
			&item.SizeBytes, &item.Width, &item.Height, &createdAtStr,
		); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTime(createdAtStr)
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) DeleteMediaItem(ctx context.Context, teamID, mediaID string) error {
	_, err := s.db.ExecContext(ctx, `delete from media_items where id = ? and team_id = ?`, mediaID, teamID)
	return err
}

func (s *Store) GetMediaProviderMapping(ctx context.Context, mediaID, accountID string) (domain.MediaProviderMapping, error) {
	var m domain.MediaProviderMapping
	var expiresAtStr sql.NullString
	var createdAtStr string
	err := s.db.QueryRowContext(ctx, `
		select media_id, account_id, remote_id, expires_at, created_at
		from media_provider_mappings
		where media_id = ? and account_id = ?
	`, mediaID, accountID).Scan(&m.MediaID, &m.AccountID, &m.RemoteID, &expiresAtStr, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.MediaProviderMapping{}, err
		}
		return domain.MediaProviderMapping{}, err
	}
	if expiresAtStr.Valid {
		t, _ := parseTime(expiresAtStr.String)
		m.ExpiresAt = &t
	}
	m.CreatedAt, _ = parseTime(createdAtStr)
	return m, nil
}

func (s *Store) UpsertMediaProviderMapping(ctx context.Context, m domain.MediaProviderMapping) error {
	now := nowString()
	var expiresAtStr *string
	if m.ExpiresAt != nil {
		s := m.ExpiresAt.Format(sqliteTimeLayout)
		expiresAtStr = &s
	}
	_, err := s.db.ExecContext(ctx, `
		insert into media_provider_mappings (media_id, account_id, remote_id, expires_at, created_at)
		values (?, ?, ?, ?, ?)
		on conflict (media_id, account_id) do update
		set remote_id = excluded.remote_id,
		    expires_at = excluded.expires_at,
		    created_at = excluded.created_at
	`, m.MediaID, m.AccountID, m.RemoteID, expiresAtStr, now)
	return err
}
