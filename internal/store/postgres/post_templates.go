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
	       media_exclude_by_account, target_account_ids, enabled, ai_enhance_enabled, ai_enhance_announcement, output_mode, prompt_hint, title_hint, tonality,
	       materialize_horizon_days, next_materialize_at, counter_next,
	       announcement_enabled, announcement_title, announcement_content, announcement_days_before,
	       announcement_counter_next, announcement_target_account_ids, created_at, updated_at
	from post_templates`

func (s *Store) ListDuePostTemplates(ctx context.Context, limit int) ([]domain.PostTemplate, error) {
	return s.ListEnabledPostTemplates(ctx, limit)
}

func (s *Store) ListEnabledPostTemplates(ctx context.Context, limit int) ([]domain.PostTemplate, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, postTemplateSelectSQL+`
		where enabled = true
		  and next_materialize_at is not null
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
	aiEnhanceAnn := false
	if input.AiEnhanceAnnouncement != nil {
		aiEnhanceAnn = *input.AiEnhanceAnnouncement
	}
	outputModeRaw := string(input.OutputMode)
	if outputModeRaw == "" {
		outputModeRaw = string(domain.AutomationOutputScheduled)
	}
	outputMode := string(domain.NormalizeAutomationOutputMode(outputModeRaw))
	promptHint := strings.TrimSpace(input.PromptHint)
	titleHint := strings.TrimSpace(input.TitleHint)
	tonality := strings.TrimSpace(input.Tonality)
	horizonDays := 0
	if input.MaterializeHorizonDays != nil {
		horizonDays = *input.MaterializeHorizonDays
		if horizonDays < 0 {
			horizonDays = 0
		}
		if horizonDays > 366 {
			horizonDays = 366
		}
	}
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
			media_exclude_by_account, target_account_ids, enabled, ai_enhance_enabled, ai_enhance_announcement, output_mode, prompt_hint, title_hint, tonality,
			materialize_horizon_days, next_materialize_at, counter_next,
			announcement_enabled, announcement_title, announcement_content, announcement_days_before,
			announcement_counter_next, announcement_target_account_ids
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
		returning `+postTemplateReturningColumns(),
		id, teamID, principal.User.ID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Content),
		strings.TrimSpace(input.RecurrenceJSON), visibility, mediaJSON, excludeJSON, targetJSON, enabled,
		aiEnhance, aiEnhanceAnn, outputMode, promptHint, titleHint, tonality, horizonDays, next, counterNext,
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
	aiEnhanceAnn := existing.AiEnhanceAnnouncement
	if input.AiEnhanceAnnouncement != nil {
		aiEnhanceAnn = *input.AiEnhanceAnnouncement
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
	horizonDays := existing.MaterializeHorizonDays
	if input.MaterializeHorizonDays != nil {
		horizonDays = *input.MaterializeHorizonDays
		if horizonDays < 0 {
			horizonDays = 0
		}
		if horizonDays > 366 {
			horizonDays = 366
		}
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
		    ai_enhance_enabled = $11, ai_enhance_announcement = $12, output_mode = $13, prompt_hint = $14, title_hint = $15, tonality = $16,
		    materialize_horizon_days = $17, next_materialize_at = $18, counter_next = $19,
		    announcement_enabled = $20, announcement_title = $21, announcement_content = $22, announcement_days_before = $23,
		    announcement_counter_next = $24, announcement_target_account_ids = $25, updated_at = now()
		where team_id = $1 and id = $2
		returning `+postTemplateReturningColumns(),
		teamID, templateID, title, content, recJSON, visibility, mediaJSON, excludeJSON, targetJSON, enabled,
		aiEnhance, aiEnhanceAnn, string(outputMode), promptHint, titleHint, tonality, horizonDays, nextAny, counterNext,
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
		select count(*) from post_template_skips
		where template_id = $1 and occurrence_at = $2
		  and (skip_scope = 'occurrence' or skip_scope = '' or skip_scope is null)`,
		templateID, occurrenceAt,
	).Scan(&n)
	return n > 0, err
}

func (s *Store) IsPostTemplateAnnouncementSkipped(ctx context.Context, templateID string, occurrenceAt time.Time) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		select count(*) from post_template_skips
		where template_id = $1 and occurrence_at = $2
		  and (skip_scope = 'announcement' or skip_scope = 'occurrence' or skip_scope = '' or skip_scope is null)`,
		templateID, occurrenceAt,
	).Scan(&n)
	return n > 0, err
}

func (s *Store) AddPostTemplateSkip(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		insert into post_template_skips (template_id, occurrence_at, skip_scope)
		select id, $3, 'occurrence' from post_templates where team_id = $1 and id = $2
		on conflict (template_id, occurrence_at) do update set skip_scope = 'occurrence'`,
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

func (s *Store) AddPostTemplateAnnouncementSkip(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		insert into post_template_skips (template_id, occurrence_at, skip_scope)
		select id, $3, 'announcement' from post_templates where team_id = $1 and id = $2
		on conflict (template_id, occurrence_at) do update set skip_scope = 'announcement'`,
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

func (s *Store) HasPostTemplateRoleMaterialized(ctx context.Context, templateID string, occurrenceAt time.Time, role string) (bool, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return false, errors.New("role is required")
	}
	var n int
	err := s.pool.QueryRow(ctx, `
		select count(*) from scheduled_posts
		where post_template_id = $1
		  and template_occurrence_at = $2
		  and template_post_role = $3
		  and status != $4`,
		templateID, occurrenceAt, role, domain.PostStatusCancelled,
	).Scan(&n)
	return n > 0, err
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
	          media_exclude_by_account, target_account_ids, enabled, ai_enhance_enabled, ai_enhance_announcement, output_mode, prompt_hint, title_hint, tonality,
	          materialize_horizon_days, next_materialize_at, counter_next,
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
		&t.AiEnhanceAnnouncement,
		&outputMode,
		&t.PromptHint,
		&t.TitleHint,
		&t.Tonality,
		&t.MaterializeHorizonDays,
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

func (s *Store) ListPostTemplateLinkedPosts(ctx context.Context, teamID, templateID string) ([]domain.PostTemplateLinkedPost, error) {
	rows, err := s.pool.Query(ctx, `
		select id::text, status, template_occurrence_at, coalesce(template_post_role, ''), template_counter
		from scheduled_posts
		where team_id = $1
		  and post_template_id = $2
		  and template_occurrence_at is not null
		  and status != $3
		order by template_occurrence_at asc, template_post_role asc`,
		teamID, templateID, domain.PostStatusCancelled,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.PostTemplateLinkedPost
	for rows.Next() {
		var item domain.PostTemplateLinkedPost
		var status string
		var occ time.Time
		var counter sql.NullInt32
		if err := rows.Scan(&item.ID, &status, &occ, &item.TemplatePostRole, &counter); err != nil {
			return nil, err
		}
		item.Status = domain.PostStatus(status)
		item.TemplateOccurrenceAt = occ.UTC()
		if counter.Valid {
			v := int(counter.Int32)
			item.TemplateCounter = &v
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) DeletePostTemplateLinkedPosts(ctx context.Context, teamID, templateID string, postIDs []string) (int, error) {
	if len(postIDs) == 0 {
		return 0, nil
	}
	tag, err := s.pool.Exec(ctx, `
		delete from scheduled_posts
		where team_id = $1
		  and post_template_id = $2
		  and id = any($3)`,
		teamID, templateID, postIDs,
	)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) SetPostTemplateMaterializationState(ctx context.Context, templateID string, nextMaterialize *time.Time, counterNext, announcementCounterNext int) error {
	var next any
	if nextMaterialize != nil {
		next = nextMaterialize.UTC()
	}
	_, err := s.pool.Exec(ctx, `
		update post_templates
		set next_materialize_at = $2,
		    counter_next = $3,
		    announcement_counter_next = $4,
		    updated_at = now()
		where id = $1`,
		templateID, next, counterNext, announcementCounterNext,
	)
	return err
}
