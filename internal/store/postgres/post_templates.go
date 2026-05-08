package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/scheduling"
	"github.com/google/uuid"
)

func (s *Store) ListDuePostTemplates(ctx context.Context, limit int) ([]domain.PostTemplate, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		select id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
		       media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next, created_at, updated_at
		from post_templates
		where enabled = true
		  and next_materialize_at is not null
		  and next_materialize_at <= now()
		order by next_materialize_at asc
		limit $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []domain.PostTemplate
	for rows.Next() {
		t, err := scanPostTemplate(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) ListPostTemplates(ctx context.Context, teamID string) ([]domain.PostTemplate, error) {
	rows, err := s.pool.Query(ctx, `
		select id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
		       media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next, created_at, updated_at
		from post_templates
		where team_id = $1
		order by created_at desc`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []domain.PostTemplate
	for rows.Next() {
		t, err := scanPostTemplate(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) GetPostTemplate(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error) {
	row := s.pool.QueryRow(ctx, `
		select id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
		       media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next, created_at, updated_at
		from post_templates
		where team_id = $1 and id = $2`,
		teamID, templateID,
	)
	t, err := scanPostTemplate(row)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	return t, nil
}

func (s *Store) CreatePostTemplate(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostTemplateInput) (domain.PostTemplate, error) {
	rule, err := scheduling.ParseRecurrenceJSON(strings.TrimSpace(input.RecurrenceJSON))
	if err != nil {
		return domain.PostTemplate{}, err
	}
	next, err := scheduling.NextOccurrence(rule, time.Now().UTC())
	if err != nil {
		return domain.PostTemplate{}, err
	}
	visibility := domain.NormalizePostVisibility(input.Visibility)
	mediaJSON, err := encodeMediaIDsJSON(input.MediaIDs)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	excludeJSON, err := encodeMediaExcludeJSON(input.MediaExcludeByAccount)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	targetJSON, err := encodeMediaIDsJSON(input.TargetAccountIDs)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	id := uuid.NewString()
	row := s.pool.QueryRow(ctx, `
		insert into post_templates (
			id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
			media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 1)
		returning id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
		          media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next, created_at, updated_at`,
		id, teamID, principal.User.ID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Content),
		strings.TrimSpace(input.RecurrenceJSON), visibility, mediaJSON, excludeJSON, targetJSON, enabled, next,
	)
	return scanPostTemplate(row)
}

func (s *Store) UpdatePostTemplate(ctx context.Context, teamID, templateID string, input domain.UpdatePostTemplateInput) (domain.PostTemplate, error) {
	existing, err := s.GetPostTemplate(ctx, teamID, templateID)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	title := existing.Title
	if input.Title != nil {
		title = strings.TrimSpace(*input.Title)
	}
	content := existing.Content
	if input.Content != nil {
		content = strings.TrimSpace(*input.Content)
	}
	recJSON := existing.RecurrenceJSON
	if input.RecurrenceJSON != nil {
		recJSON = strings.TrimSpace(*input.RecurrenceJSON)
	}
	visibility := existing.Visibility
	if input.Visibility != nil {
		visibility = domain.NormalizePostVisibility(*input.Visibility)
	}
	enabled := existing.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	mediaIDs := existing.MediaIDs
	if input.MediaIDs != nil {
		mediaIDs = *input.MediaIDs
	}
	ex := existing.MediaExcludeByAccount
	if input.MediaExcludeByAccount != nil {
		ex = input.MediaExcludeByAccount
	}
	targets := existing.TargetAccountIDs
	if input.TargetAccountIDs != nil {
		targets = *input.TargetAccountIDs
	}

	rule, err := scheduling.ParseRecurrenceJSON(recJSON)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	var next *time.Time
	if !enabled {
		next = nil
	} else if input.RecurrenceJSON != nil || (input.Enabled != nil && *input.Enabled && !existing.Enabled) {
		nx, err := scheduling.NextOccurrence(rule, time.Now().UTC())
		if err != nil {
			return domain.PostTemplate{}, err
		}
		next = &nx
	} else {
		next = existing.NextMaterializeAt
	}

	mediaJSON, err := encodeMediaIDsJSON(mediaIDs)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	excludeJSON, err := encodeMediaExcludeJSON(ex)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	targetJSON, err := encodeMediaIDsJSON(targets)
	if err != nil {
		return domain.PostTemplate{}, err
	}

	var nextAny any
	if next != nil {
		nextAny = *next
	}

	row := s.pool.QueryRow(ctx, `
		update post_templates
		set title = $3, content = $4, recurrence_json = $5, visibility = $6, media_ids = $7,
		    media_exclude_by_account = $8, target_account_ids = $9, enabled = $10, next_materialize_at = $11, updated_at = now()
		where team_id = $1 and id = $2
		returning id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
		          media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next, created_at, updated_at`,
		teamID, templateID, title, content, recJSON, visibility, mediaJSON, excludeJSON, targetJSON, enabled, nextAny,
	)
	return scanPostTemplate(row)
}

func (s *Store) DeletePostTemplate(ctx context.Context, teamID, templateID string) error {
	tag, err := s.pool.Exec(ctx, `delete from post_templates where team_id = $1 and id = $2`, teamID, templateID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("template not found")
	}
	return nil
}

func (s *Store) IsPostTemplateOccurrenceSkipped(ctx context.Context, templateID string, occurrenceAt time.Time) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		select count(*) from post_template_skips where template_id = $1 and occurrence_at = $2`,
		templateID, occurrenceAt,
	).Scan(&n)
	return n > 0, err
}

func (s *Store) AddPostTemplateSkip(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		insert into post_template_skips (template_id, occurrence_at)
		select id, $3 from post_templates where team_id = $1 and id = $2`,
		teamID, templateID, occurrenceAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("template not found")
	}
	return nil
}

func (s *Store) AdvancePostTemplateSchedule(ctx context.Context, templateID string, nextMaterialize *time.Time, counterNext int) error {
	var next any
	if nextMaterialize != nil {
		next = *nextMaterialize
	}
	_, err := s.pool.Exec(ctx, `
		update post_templates
		set next_materialize_at = $2, counter_next = $3, updated_at = now()
		where id = $1`,
		templateID, next, counterNext,
	)
	return err
}

func scanPostTemplate(row interface {
	Scan(dest ...any) error
}) (domain.PostTemplate, error) {
	var t domain.PostTemplate
	var mediaRaw, excludeRaw, targetRaw string
	var next sql.NullTime
	err := row.Scan(
		&t.ID,
		&t.TeamID,
		&t.AuthorUserID,
		&t.Title,
		&t.Content,
		&t.RecurrenceJSON,
		&t.Visibility,
		&mediaRaw,
		&excludeRaw,
		&targetRaw,
		&t.Enabled,
		&next,
		&t.CounterNext,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	if next.Valid {
		u := next.Time.UTC()
		t.NextMaterializeAt = &u
	}
	if err := decodePostMediaIDs(mediaRaw, &t.MediaIDs); err != nil {
		return domain.PostTemplate{}, err
	}
	if err := decodePostMediaExclude(excludeRaw, &t.MediaExcludeByAccount); err != nil {
		return domain.PostTemplate{}, err
	}
	if err := decodePostMediaIDs(targetRaw, &t.TargetAccountIDs); err != nil {
		return domain.PostTemplate{}, err
	}
	return t, nil
}
