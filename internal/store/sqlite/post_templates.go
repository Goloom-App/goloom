package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
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
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
		       media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next,
		       announces_template_id, announcement_days_before, created_at, updated_at
		from post_templates
		where enabled = 1
		  and next_materialize_at is not null
		  and next_materialize_at <= ?
		order by next_materialize_at asc
		limit ?`,
		nowString(), limit,
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
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
		       media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next,
		       announces_template_id, announcement_days_before, created_at, updated_at
		from post_templates
		where team_id = ?
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
	row := s.db.QueryRowContext(ctx, `
		select id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
		       media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next,
		       announces_template_id, announcement_days_before, created_at, updated_at
		from post_templates
		where team_id = ? and id = ?`,
		teamID, templateID,
	)
	return scanPostTemplate(row)
}

func (s *Store) CreatePostTemplate(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostTemplateInput) (domain.PostTemplate, error) {
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
	enabled := 1
	if input.Enabled != nil && !*input.Enabled {
		enabled = 0
	}
	isAnnouncement := input.AnnouncesTemplateID != nil && *input.AnnouncesTemplateID != ""
	var nextStr any
	if !isAnnouncement {
		rule, err := scheduling.ParseRecurrenceJSON(strings.TrimSpace(input.RecurrenceJSON))
		if err != nil {
			return domain.PostTemplate{}, err
		}
		nx, err := scheduling.NextOccurrence(rule, time.Now().UTC())
		if err != nil {
			return domain.PostTemplate{}, err
		}
		nextStr = formatTime(nx)
	}
	id := uuid.NewString()
	now := nowString()
	var announcesID, annDays any
	if isAnnouncement {
		announcesID = *input.AnnouncesTemplateID
	}
	if input.AnnouncementDaysBefore != nil {
		annDays = *input.AnnouncementDaysBefore
	}
	_, err = s.db.ExecContext(ctx, `
		insert into post_templates (
			id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
			media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next,
			announces_template_id, announcement_days_before, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)`,
		id, teamID, principal.User.ID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Content),
		strings.TrimSpace(input.RecurrenceJSON), visibility, mediaJSON, excludeJSON, targetJSON, enabled,
		nextStr, announcesID, annDays, now, now,
	)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	return s.GetPostTemplate(ctx, teamID, id)
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

	announcesID := existing.AnnouncesTemplateID
	if input.AnnouncesTemplateID != nil {
		if *input.AnnouncesTemplateID == "" {
			announcesID = nil
		} else {
			announcesID = input.AnnouncesTemplateID
		}
	}
	isAnnouncement := announcesID != nil && *announcesID != ""

	var next *time.Time
	if isAnnouncement {
		next = nil
	} else if !enabled {
		next = nil
	} else {
		rule, err := scheduling.ParseRecurrenceJSON(recJSON)
		if err != nil {
			return domain.PostTemplate{}, err
		}
		if input.RecurrenceJSON != nil || (input.Enabled != nil && *input.Enabled && !existing.Enabled) {
			nx, err := scheduling.NextOccurrence(rule, time.Now().UTC())
			if err != nil {
				return domain.PostTemplate{}, err
			}
			next = &nx
		} else {
			next = existing.NextMaterializeAt
		}
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

	en := 0
	if enabled {
		en = 1
	}
	var nextStr any
	if next != nil {
		nextStr = formatTime(*next)
	}

	var announcesIDAny, annDaysAny any
	if announcesID != nil {
		announcesIDAny = *announcesID
	}
	if input.AnnouncementDaysBefore != nil {
		annDaysAny = *input.AnnouncementDaysBefore
	} else if existing.AnnouncementDaysBefore != nil {
		annDaysAny = *existing.AnnouncementDaysBefore
	}

	_, err = s.db.ExecContext(ctx, `
		update post_templates
		set title = ?, content = ?, recurrence_json = ?, visibility = ?, media_ids = ?,
		    media_exclude_by_account = ?, target_account_ids = ?, enabled = ?, next_materialize_at = ?,
		    announces_template_id = ?, announcement_days_before = ?, updated_at = ?
		where team_id = ? and id = ?`,
		title, content, recJSON, visibility, mediaJSON, excludeJSON, targetJSON, en, nextStr,
		announcesIDAny, annDaysAny, nowString(), teamID, templateID,
	)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	return s.GetPostTemplate(ctx, teamID, templateID)
}

func (s *Store) DeletePostTemplate(ctx context.Context, teamID, templateID string) error {
	res, err := s.db.ExecContext(ctx, `delete from post_templates where team_id = ? and id = ?`, teamID, templateID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("template not found")
	}
	return nil
}

func (s *Store) IsPostTemplateOccurrenceSkipped(ctx context.Context, templateID string, occurrenceAt time.Time) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		select count(*) from post_template_skips where template_id = ? and occurrence_at = ?`,
		templateID, formatTime(occurrenceAt),
	).Scan(&n)
	return n > 0, err
}

func (s *Store) AddPostTemplateSkip(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		insert into post_template_skips (template_id, occurrence_at)
		select id, ? from post_templates where team_id = ? and id = ?`,
		formatTime(occurrenceAt), teamID, templateID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("template not found")
	}
	return nil
}

func (s *Store) ShiftPostTemplateOccurrence(ctx context.Context, teamID, templateID string, occurrenceAt, shiftTo time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		insert into post_template_skips (template_id, occurrence_at, shift_to)
		select id, ?, ? from post_templates where team_id = ? and id = ?`,
		formatTime(occurrenceAt), formatTime(shiftTo), teamID, templateID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("template not found")
	}
	return nil
}

func (s *Store) GetPostTemplateShiftTo(ctx context.Context, templateID string, occurrenceAt time.Time) (*time.Time, error) {
	var shiftStr sql.NullString
	err := s.db.QueryRowContext(ctx, `
		select shift_to from post_template_skips where template_id = ? and occurrence_at = ?`,
		templateID, formatTime(occurrenceAt),
	).Scan(&shiftStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !shiftStr.Valid || strings.TrimSpace(shiftStr.String) == "" {
		return nil, nil
	}
	parsed, err := parseTime(shiftStr.String)
	if err != nil {
		return nil, err
	}
	u := parsed.UTC()
	return &u, nil
}

func (s *Store) ListAnnouncingTemplates(ctx context.Context, parentTemplateID string) ([]domain.PostTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
		       media_exclude_by_account, target_account_ids, enabled, next_materialize_at, counter_next,
		       announces_template_id, announcement_days_before, created_at, updated_at
		from post_templates
		where announces_template_id = ? and enabled = 1`,
		parentTemplateID,
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

func (s *Store) AdvancePostTemplateSchedule(ctx context.Context, templateID string, nextMaterialize *time.Time, counterNext int) error {
	var next any
	if nextMaterialize != nil {
		next = formatTime(*nextMaterialize)
	}
	_, err := s.db.ExecContext(ctx, `
		update post_templates
		set next_materialize_at = ?, counter_next = ?, updated_at = ?
		where id = ?`,
		next, counterNext, nowString(), templateID,
	)
	return err
}

func scanPostTemplate(row interface {
	Scan(dest ...any) error
}) (domain.PostTemplate, error) {
	var t domain.PostTemplate
	var mediaRaw, excludeRaw, targetRaw string
	var next sql.NullString
	var enabledInt int
	var createdAt, updatedAt string
	var announcesID sql.NullString
	var annDays sql.NullInt64
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
		&enabledInt,
		&next,
		&t.CounterNext,
		&announcesID,
		&annDays,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	t.Enabled = enabledInt != 0
	if announcesID.Valid && strings.TrimSpace(announcesID.String) != "" {
		t.AnnouncesTemplateID = &announcesID.String
	}
	if annDays.Valid {
		v := int(annDays.Int64)
		t.AnnouncementDaysBefore = &v
	}
	if next.Valid && strings.TrimSpace(next.String) != "" {
		parsed, err := parseTime(next.String)
		if err != nil {
			return domain.PostTemplate{}, err
		}
		u := parsed.UTC()
		t.NextMaterializeAt = &u
	}
	if strings.TrimSpace(mediaRaw) != "" {
		if err := json.Unmarshal([]byte(mediaRaw), &t.MediaIDs); err != nil {
			return domain.PostTemplate{}, err
		}
	}
	if strings.TrimSpace(excludeRaw) != "" && excludeRaw != "{}" {
		if err := json.Unmarshal([]byte(excludeRaw), &t.MediaExcludeByAccount); err != nil {
			return domain.PostTemplate{}, err
		}
	}
	if err := json.Unmarshal([]byte(targetRaw), &t.TargetAccountIDs); err != nil {
		return domain.PostTemplate{}, err
	}
	var err2 error
	t.CreatedAt, err2 = parseTime(createdAt)
	if err2 != nil {
		return domain.PostTemplate{}, err2
	}
	t.UpdatedAt, err2 = parseTime(updatedAt)
	return t, err2
}
