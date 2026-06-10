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

const postTemplateSelectSQL = `
	select id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
	       media_exclude_by_account, target_account_ids, enabled, ai_enhance_enabled, output_mode, prompt_hint, title_hint, tonality,
	       next_materialize_at, counter_next,
	       announcement_enabled, announcement_title, announcement_content, announcement_days_before,
	       announcement_counter_next, announcement_target_account_ids, created_at, updated_at
	from post_templates`

func (s *Store) ListDuePostTemplates(ctx context.Context, limit int) ([]domain.PostTemplate, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, postTemplateSelectSQL+`
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
	rows, err := s.db.QueryContext(ctx, postTemplateSelectSQL+`
		where team_id = ?
		  and (announces_template_id is null or trim(announces_template_id) = '')
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
	row := s.db.QueryRowContext(ctx, postTemplateSelectSQL+`
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
	rule, err := scheduling.ParseRecurrenceJSON(strings.TrimSpace(input.RecurrenceJSON))
	if err != nil {
		return domain.PostTemplate{}, err
	}
	nx, err := scheduling.NextOccurrence(rule, time.Now().UTC())
	if err != nil {
		return domain.PostTemplate{}, err
	}
	nextStr := formatTime(nx)

	aiEnhance := 0
	if input.AiEnhanceEnabled != nil && *input.AiEnhanceEnabled {
		aiEnhance = 1
	}
	outputModeRaw := string(input.OutputMode)
	if outputModeRaw == "" {
		outputModeRaw = string(domain.AutomationOutputScheduled)
	}
	outputMode := string(domain.NormalizeAutomationOutputMode(outputModeRaw))
	promptHint := strings.TrimSpace(input.PromptHint)
	titleHint := strings.TrimSpace(input.TitleHint)
	tonality := strings.TrimSpace(input.Tonality)
	counterNext := 1
	if input.CounterNext != nil && *input.CounterNext >= 1 {
		counterNext = *input.CounterNext
	}

	annEnabled := 0
	if input.AnnouncementEnabled != nil && *input.AnnouncementEnabled {
		annEnabled = 1
	}
	annTitle := strings.TrimSpace(input.AnnouncementTitle)
	annContent := strings.TrimSpace(input.AnnouncementContent)
	annDays := 0
	if input.AnnouncementDaysBefore != nil {
		annDays = *input.AnnouncementDaysBefore
	}
	annCounter := 1
	if input.AnnouncementCounterNext != nil && *input.AnnouncementCounterNext >= 1 {
		annCounter = *input.AnnouncementCounterNext
	}
	annTargetsJSON, err := encodeMediaIDsJSON(domain.NormalizeMediaIDs(input.AnnouncementTargetAccountIDs))
	if err != nil {
		return domain.PostTemplate{}, err
	}

	id := uuid.NewString()
	now := nowString()
	_, err = s.db.ExecContext(ctx, `
		insert into post_templates (
			id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
			media_exclude_by_account, target_account_ids, enabled, ai_enhance_enabled, output_mode, prompt_hint, title_hint, tonality,
			next_materialize_at, counter_next,
			announcement_enabled, announcement_title, announcement_content, announcement_days_before,
			announcement_counter_next, announcement_target_account_ids, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, teamID, principal.User.ID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Content),
		strings.TrimSpace(input.RecurrenceJSON), visibility, mediaJSON, excludeJSON, targetJSON, enabled,
		aiEnhance, outputMode, promptHint, titleHint, tonality,
		nextStr, counterNext,
		annEnabled, annTitle, annContent, annDays, annCounter, annTargetsJSON, now, now,
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

	var next *time.Time
	if !enabled {
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

	aiEnhance := 0
	if existing.AiEnhanceEnabled {
		aiEnhance = 1
	}
	if input.AiEnhanceEnabled != nil {
		aiEnhance = 0
		if *input.AiEnhanceEnabled {
			aiEnhance = 1
		}
	}
	outputMode := existing.OutputMode
	if input.OutputMode != nil {
		outputMode = domain.NormalizeAutomationOutputMode(string(*input.OutputMode))
	}
	promptHint := existing.PromptHint
	if input.PromptHint != nil {
		promptHint = strings.TrimSpace(*input.PromptHint)
	}
	titleHint := existing.TitleHint
	if input.TitleHint != nil {
		titleHint = strings.TrimSpace(*input.TitleHint)
	}
	tonality := existing.Tonality
	if input.Tonality != nil {
		tonality = strings.TrimSpace(*input.Tonality)
	}
	counterNext := existing.CounterNext
	if input.CounterNext != nil && *input.CounterNext >= 1 {
		counterNext = *input.CounterNext
	}

	annEnabled := existing.AnnouncementEnabled
	if input.AnnouncementEnabled != nil {
		annEnabled = *input.AnnouncementEnabled
	}
	annTitle := existing.AnnouncementTitle
	if input.AnnouncementTitle != nil {
		annTitle = strings.TrimSpace(*input.AnnouncementTitle)
	}
	annContent := existing.AnnouncementContent
	if input.AnnouncementContent != nil {
		annContent = strings.TrimSpace(*input.AnnouncementContent)
	}
	annDays := existing.AnnouncementDaysBefore
	if input.AnnouncementDaysBefore != nil {
		annDays = *input.AnnouncementDaysBefore
	}
	annCounter := existing.AnnouncementCounterNext
	if input.AnnouncementCounterNext != nil && *input.AnnouncementCounterNext >= 1 {
		annCounter = *input.AnnouncementCounterNext
	}
	annTargets := existing.AnnouncementTargetAccountIDs
	if input.AnnouncementTargetAccountIDs != nil {
		annTargets = *input.AnnouncementTargetAccountIDs
	}
	annTargetsJSON, err := encodeMediaIDsJSON(annTargets)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	annEnInt := 0
	if annEnabled {
		annEnInt = 1
	}

	_, err = s.db.ExecContext(ctx, `
		update post_templates
		set title = ?, content = ?, recurrence_json = ?, visibility = ?, media_ids = ?,
		    media_exclude_by_account = ?, target_account_ids = ?, enabled = ?,
		    ai_enhance_enabled = ?, output_mode = ?, prompt_hint = ?, title_hint = ?, tonality = ?,
		    next_materialize_at = ?, counter_next = ?,
		    announcement_enabled = ?, announcement_title = ?, announcement_content = ?, announcement_days_before = ?,
		    announcement_counter_next = ?, announcement_target_account_ids = ?, updated_at = ?
		where team_id = ? and id = ?`,
		title, content, recJSON, visibility, mediaJSON, excludeJSON, targetJSON, en,
		aiEnhance, string(outputMode), promptHint, titleHint, tonality, nextStr, counterNext,
		annEnInt, annTitle, annContent, annDays, annCounter, annTargetsJSON, nowString(), teamID, templateID,
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

func (s *Store) AdvancePostTemplateAnnouncementCounter(ctx context.Context, templateID string, counterNext int) error {
	_, err := s.db.ExecContext(ctx, `
		update post_templates
		set announcement_counter_next = ?, updated_at = ?
		where id = ?`,
		counterNext, nowString(), templateID,
	)
	return err
}

func scanPostTemplate(row interface {
	Scan(dest ...any) error
}) (domain.PostTemplate, error) {
	var t domain.PostTemplate
	var mediaRaw, excludeRaw, targetRaw, annTargetsRaw string
	var next sql.NullString
	var enabledInt int
	var aiEnhanceInt int
	var annEnabledInt int
	var outputMode string
	var createdAt, updatedAt string
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
		&aiEnhanceInt,
		&outputMode,
		&t.PromptHint,
		&t.TitleHint,
		&t.Tonality,
		&next,
		&t.CounterNext,
		&annEnabledInt,
		&t.AnnouncementTitle,
		&t.AnnouncementContent,
		&t.AnnouncementDaysBefore,
		&t.AnnouncementCounterNext,
		&annTargetsRaw,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	t.Enabled = enabledInt != 0
	t.AiEnhanceEnabled = aiEnhanceInt != 0
	t.AnnouncementEnabled = annEnabledInt != 0
	t.OutputMode = domain.NormalizeAutomationOutputMode(outputMode)
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
	if strings.TrimSpace(annTargetsRaw) != "" && annTargetsRaw != "[]" {
		if err := json.Unmarshal([]byte(annTargetsRaw), &t.AnnouncementTargetAccountIDs); err != nil {
			return domain.PostTemplate{}, err
		}
	}
	var err2 error
	t.CreatedAt, err2 = parseTime(createdAt)
	if err2 != nil {
		return domain.PostTemplate{}, err2
	}
	t.UpdatedAt, err2 = parseTime(updatedAt)
	return t, err2
}
