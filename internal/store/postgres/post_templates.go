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
	rows, err := s.pool.Query(ctx, postTemplateSelectSQL+`
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
	rows, err := s.pool.Query(ctx, postTemplateSelectSQL+`
		where team_id = $1
		  and announces_template_id is null
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
	row := s.pool.QueryRow(ctx, postTemplateSelectSQL+`
		where team_id = $1 and id = $2`,
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
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	rule, err := scheduling.ParseRecurrenceJSON(strings.TrimSpace(input.RecurrenceJSON))
	if err != nil {
		return domain.PostTemplate{}, err
	}
	next, err := scheduling.NextOccurrence(rule, time.Now().UTC())
	if err != nil {
		return domain.PostTemplate{}, err
	}

	aiEnhance := false
	if input.AiEnhanceEnabled != nil {
		aiEnhance = *input.AiEnhanceEnabled
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

	annEnabled := false
	if input.AnnouncementEnabled != nil {
		annEnabled = *input.AnnouncementEnabled
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
	row := s.pool.QueryRow(ctx, `
		insert into post_templates (
			id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
			media_exclude_by_account, target_account_ids, enabled, ai_enhance_enabled, output_mode, prompt_hint, title_hint, tonality,
			next_materialize_at, counter_next,
			announcement_enabled, announcement_title, announcement_content, announcement_days_before,
			announcement_counter_next, announcement_target_account_ids
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
		returning `+postTemplateReturningColumns(),
		id, teamID, principal.User.ID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Content),
		strings.TrimSpace(input.RecurrenceJSON), visibility, mediaJSON, excludeJSON, targetJSON, enabled,
		aiEnhance, outputMode, promptHint, titleHint, tonality, next, counterNext,
		annEnabled, annTitle, annContent, annDays, annCounter, annTargetsJSON,
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

	var nextAny any
	if next != nil {
		nextAny = *next
	}

	aiEnhance := existing.AiEnhanceEnabled
	if input.AiEnhanceEnabled != nil {
		aiEnhance = *input.AiEnhanceEnabled
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

	row := s.pool.QueryRow(ctx, `
		update post_templates
		set title = $3, content = $4, recurrence_json = $5, visibility = $6, media_ids = $7,
		    media_exclude_by_account = $8, target_account_ids = $9, enabled = $10,
		    ai_enhance_enabled = $11, output_mode = $12, prompt_hint = $13, title_hint = $14, tonality = $15,
		    next_materialize_at = $16, counter_next = $17,
		    announcement_enabled = $18, announcement_title = $19, announcement_content = $20, announcement_days_before = $21,
		    announcement_counter_next = $22, announcement_target_account_ids = $23, updated_at = now()
		where team_id = $1 and id = $2
		returning `+postTemplateReturningColumns(),
		teamID, templateID, title, content, recJSON, visibility, mediaJSON, excludeJSON, targetJSON, enabled,
		aiEnhance, string(outputMode), promptHint, titleHint, tonality, nextAny, counterNext,
		annEnabled, annTitle, annContent, annDays, annCounter, annTargetsJSON,
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

func (s *Store) ShiftPostTemplateOccurrence(ctx context.Context, teamID, templateID string, occurrenceAt, shiftTo time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		insert into post_template_skips (template_id, occurrence_at, shift_to)
		select id, $3, $4 from post_templates where team_id = $1 and id = $2`,
		teamID, templateID, occurrenceAt, shiftTo,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("template not found")
	}
	return nil
}

func (s *Store) GetPostTemplateShiftTo(ctx context.Context, templateID string, occurrenceAt time.Time) (*time.Time, error) {
	var shift sql.NullTime
	err := s.pool.QueryRow(ctx, `
		select shift_to from post_template_skips where template_id = $1 and occurrence_at = $2`,
		templateID, occurrenceAt,
	).Scan(&shift)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !shift.Valid {
		return nil, nil
	}
	u := shift.Time.UTC()
	return &u, nil
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

func (s *Store) AdvancePostTemplateAnnouncementCounter(ctx context.Context, templateID string, counterNext int) error {
	_, err := s.pool.Exec(ctx, `
		update post_templates
		set announcement_counter_next = $2, updated_at = now()
		where id = $1`,
		templateID, counterNext,
	)
	return err
}

func postTemplateReturningColumns() string {
	return `id, team_id, author_user_id, title, content, recurrence_json, visibility, media_ids,
	          media_exclude_by_account, target_account_ids, enabled, ai_enhance_enabled, output_mode, prompt_hint, title_hint, tonality,
	          next_materialize_at, counter_next,
	          announcement_enabled, announcement_title, announcement_content, announcement_days_before,
	          announcement_counter_next, announcement_target_account_ids, created_at, updated_at`
}

func scanPostTemplate(row interface {
	Scan(dest ...any) error
}) (domain.PostTemplate, error) {
	var t domain.PostTemplate
	var mediaRaw, excludeRaw, targetRaw, annTargetsRaw string
	var next sql.NullTime
	var outputMode string
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
		&t.AiEnhanceEnabled,
		&outputMode,
		&t.PromptHint,
		&t.TitleHint,
		&t.Tonality,
		&next,
		&t.CounterNext,
		&t.AnnouncementEnabled,
		&t.AnnouncementTitle,
		&t.AnnouncementContent,
		&t.AnnouncementDaysBefore,
		&t.AnnouncementCounterNext,
		&annTargetsRaw,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		return domain.PostTemplate{}, err
	}
	t.OutputMode = domain.NormalizeAutomationOutputMode(outputMode)
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
	if strings.TrimSpace(annTargetsRaw) != "" && annTargetsRaw != "[]" {
		if err := decodePostMediaIDs(annTargetsRaw, &t.AnnouncementTargetAccountIDs); err != nil {
			return domain.PostTemplate{}, err
		}
	}
	return t, nil
}
