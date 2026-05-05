package postgres

import (
	"context"
	"errors"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) FindMediaItemByTeamSHA256(ctx context.Context, teamID, sha256 string) (domain.MediaItem, bool, error) {
	var item domain.MediaItem
	err := s.pool.QueryRow(ctx, `
		select id, team_id, sha256, filename, mime_type, size_bytes, width, height, created_at
		from media_items
		where team_id = $1 and sha256 = $2
		order by created_at asc
		limit 1
	`, teamID, sha256).Scan(
		&item.ID, &item.TeamID, &item.Sha256, &item.Filename, &item.MimeType,
		&item.SizeBytes, &item.Width, &item.Height, &item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.MediaItem{}, false, nil
		}
		return domain.MediaItem{}, false, err
	}
	return item, true, nil
}

func (s *Store) CreateMediaItem(ctx context.Context, item domain.MediaItem) (domain.MediaItem, error) {
	err := s.pool.QueryRow(ctx, `
		insert into media_items (team_id, sha256, filename, mime_type, size_bytes, width, height)
		values ($1, $2, $3, $4, $5, $6, $7)
		returning id, created_at
	`, item.TeamID, item.Sha256, item.Filename, item.MimeType, item.SizeBytes, item.Width, item.Height).Scan(&item.ID, &item.CreatedAt)
	return item, err
}

func (s *Store) GetMediaItemByID(ctx context.Context, teamID, mediaID string) (domain.MediaItem, error) {
	var item domain.MediaItem
	err := s.pool.QueryRow(ctx, `
		select id, team_id, sha256, filename, mime_type, size_bytes, width, height, created_at
		from media_items
		where id = $1 and team_id = $2
	`, mediaID, teamID).Scan(
		&item.ID, &item.TeamID, &item.Sha256, &item.Filename, &item.MimeType,
		&item.SizeBytes, &item.Width, &item.Height, &item.CreatedAt,
	)
	return item, err
}

func (s *Store) ListTeamMedia(ctx context.Context, teamID string) ([]domain.MediaItem, error) {
	rows, err := s.pool.Query(ctx, `
		select id, team_id, sha256, filename, mime_type, size_bytes, width, height, created_at
		from media_items
		where team_id = $1
		order by created_at desc
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.MediaItem
	for rows.Next() {
		var item domain.MediaItem
		if err := rows.Scan(
			&item.ID, &item.TeamID, &item.Sha256, &item.Filename, &item.MimeType,
			&item.SizeBytes, &item.Width, &item.Height, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) DeleteMediaItem(ctx context.Context, teamID, mediaID string) error {
	_, err := s.pool.Exec(ctx, `delete from media_items where id = $1 and team_id = $2`, mediaID, teamID)
	return err
}

func (s *Store) GetMediaProviderMapping(ctx context.Context, mediaID, accountID string) (domain.MediaProviderMapping, error) {
	var m domain.MediaProviderMapping
	err := s.pool.QueryRow(ctx, `
		select media_id, account_id, remote_id, expires_at, created_at
		from media_provider_mappings
		where media_id = $1 and account_id = $2
	`, mediaID, accountID).Scan(&m.MediaID, &m.AccountID, &m.RemoteID, &m.ExpiresAt, &m.CreatedAt)
	return m, err
}

func (s *Store) UpsertMediaProviderMapping(ctx context.Context, m domain.MediaProviderMapping) error {
	_, err := s.pool.Exec(ctx, `
		insert into media_provider_mappings (media_id, account_id, remote_id, expires_at)
		values ($1, $2, $3, $4)
		on conflict (media_id, account_id) do update
		set remote_id = excluded.remote_id,
		    expires_at = excluded.expires_at,
		    created_at = now()
	`, m.MediaID, m.AccountID, m.RemoteID, m.ExpiresAt)
	return err
}
