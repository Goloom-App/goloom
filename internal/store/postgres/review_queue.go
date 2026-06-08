package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) ListAutomationReviewDrafts(ctx context.Context, teamID string, limit int) ([]domain.ReviewQueueItem, error) {
	if limit <= 0 {
		limit = 200
	}
	const query = `
		select p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status, p.source,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       p.visibility, p.media_ids, coalesce(p.media_exclude_by_account::text, '{}'),
		       p.post_template_id::text, p.template_counter,
		       coalesce(array_agg(t.account_id::text) filter (where t.account_id is not null), '{}'),
		       coalesce(max(f.name), '')
		from scheduled_posts p
		left join scheduled_post_targets t on t.post_id = p.id
		left join rss_feed_configs f on f.id = p.rss_feed_id
		where p.team_id = $1
		  and p.status = $2
		  and p.source = $3
		group by p.id
		order by p.scheduled_at asc
		limit $4
	`
	rows, err := s.pool.Query(ctx, query, teamID, domain.PostStatusDraft, domain.PostSourceAutomation, limit)
	if err != nil {
		return nil, fmt.Errorf("ListAutomationReviewDrafts: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	var items []domain.ReviewQueueItem
	for rows.Next() {
		post, feedName, err := scanReviewQueueRow(rows)
		if err != nil {
			return nil, fmt.Errorf("ListAutomationReviewDrafts: scan: %w", err)
		}
		items = append(items, domain.NewReviewQueueItem(post, feedName, now))
	}
	return items, rows.Err()
}

func scanReviewQueueRow(rows interface {
	Scan(dest ...any) error
}) (domain.ScheduledPost, string, error) {
	post := domain.ScheduledPost{}
	var mediaRaw, mediaExcludeRaw string
	var postTemplateID sql.NullString
	var templateCtr sql.NullInt64
	var feedName string
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
		&feedName,
	); err != nil {
		return domain.ScheduledPost{}, "", err
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
	if err := decodePostMediaIDs(mediaRaw, &post.MediaIDs); err != nil {
		return domain.ScheduledPost{}, "", err
	}
	if err := decodePostMediaExclude(mediaExcludeRaw, &post.MediaExcludeByAccount); err != nil {
		return domain.ScheduledPost{}, "", err
	}
	return post, feedName, nil
}
