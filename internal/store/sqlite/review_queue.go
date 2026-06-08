package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) ListAutomationReviewDrafts(ctx context.Context, teamID string, limit int) ([]domain.ReviewQueueItem, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, title, content, scheduled_at, status, source,
		       attempt_count, last_error, visibility, media_ids, media_exclude_by_account,
		       post_template_id, template_counter, rss_feed_id, created_at, updated_at
		from scheduled_posts
		where team_id = ?
		  and status = ?
		  and source = ?
		order by scheduled_at asc
		limit ?`,
		teamID, domain.PostStatusDraft, domain.PostSourceAutomation, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ListAutomationReviewDrafts: %w", err)
	}
	defer rows.Close()

	type row struct {
		post      domain.ScheduledPost
		rssFeedID sql.NullString
	}
	var collected []row
	for rows.Next() {
		item, rssFeedID, err := scanReviewPostRow(rows)
		if err != nil {
			return nil, fmt.Errorf("ListAutomationReviewDrafts: scan: %w", err)
		}
		collected = append(collected, row{post: item, rssFeedID: rssFeedID})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(collected) == 0 {
		return nil, nil
	}

	posts := make([]domain.ScheduledPost, len(collected))
	for i := range collected {
		posts[i] = collected[i].post
	}
	if err := s.attachTargetAccounts(ctx, posts); err != nil {
		return nil, err
	}

	feedNames := make(map[string]string)
	for _, item := range collected {
		if !item.rssFeedID.Valid || item.rssFeedID.String == "" {
			continue
		}
		feedID := item.rssFeedID.String
		if _, ok := feedNames[feedID]; ok {
			continue
		}
		var name string
		err := s.db.QueryRowContext(ctx, `select name from rss_feed_configs where id = ?`, feedID).Scan(&name)
		if err == nil {
			feedNames[feedID] = name
		}
	}

	now := time.Now().UTC()
	items := make([]domain.ReviewQueueItem, len(collected))
	for i := range collected {
		feedName := ""
		if collected[i].rssFeedID.Valid {
			feedName = feedNames[collected[i].rssFeedID.String]
		}
		items[i] = domain.NewReviewQueueItem(posts[i], feedName, now)
	}
	return items, nil
}

func scanReviewPostRow(rows *sql.Rows) (domain.ScheduledPost, sql.NullString, error) {
	var (
		post            domain.ScheduledPost
		scheduledAt     string
		createdAt       string
		updatedAt       string
		mediaRaw        string
		mediaExcludeRaw string
		lastError       sql.NullString
		postTemplateID  sql.NullString
		templateCtr     sql.NullInt64
		rssFeedID       sql.NullString
	)
	if err := rows.Scan(
		&post.ID,
		&post.TeamID,
		&post.AuthorUserID,
		&post.Title,
		&post.Content,
		&scheduledAt,
		&post.Status,
		&post.Source,
		&post.AttemptCount,
		&lastError,
		&post.Visibility,
		&mediaRaw,
		&mediaExcludeRaw,
		&postTemplateID,
		&templateCtr,
		&rssFeedID,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.ScheduledPost{}, sql.NullString{}, err
	}
	post.LastError = lastError.String
	var err error
	post.ScheduledAt, err = parseTime(scheduledAt)
	if err != nil {
		return domain.ScheduledPost{}, sql.NullString{}, err
	}
	post.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.ScheduledPost{}, sql.NullString{}, err
	}
	post.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.ScheduledPost{}, sql.NullString{}, err
	}
	if postTemplateID.Valid {
		s := postTemplateID.String
		post.PostTemplateID = &s
	}
	if templateCtr.Valid {
		v := int(templateCtr.Int64)
		post.TemplateCounter = &v
	}
	if strings.TrimSpace(mediaRaw) != "" {
		if err := json.Unmarshal([]byte(mediaRaw), &post.MediaIDs); err != nil {
			return domain.ScheduledPost{}, sql.NullString{}, fmt.Errorf("decode media_ids: %w", err)
		}
	}
	if strings.TrimSpace(mediaExcludeRaw) != "" && mediaExcludeRaw != "{}" {
		if err := json.Unmarshal([]byte(mediaExcludeRaw), &post.MediaExcludeByAccount); err != nil {
			return domain.ScheduledPost{}, sql.NullString{}, fmt.Errorf("decode media_exclude_by_account: %w", err)
		}
	}
	if strings.TrimSpace(string(post.Source)) == "" {
		post.Source = domain.PostSourceScheduled
	}
	return post, rssFeedID, nil
}
