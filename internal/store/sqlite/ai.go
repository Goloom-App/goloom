package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) CreateTeamProfile(ctx context.Context, teamID string, input domain.TeamProfile) (domain.TeamProfile, error) {
	metaJSON, err := json.Marshal(input.StyleMetadata)
	if err != nil {
		return domain.TeamProfile{}, fmt.Errorf("marshal style_metadata: %w", err)
	}
	id := uuid.NewString()
	now := nowString()
	_, err = s.db.ExecContext(ctx, `
		insert into team_profiles (id, team_id, style_metadata, auto_publish_enabled, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?)`,
		id, teamID, string(metaJSON), boolToInt(input.AutoPublishEnabled), now, now,
	)
	if err != nil {
		return domain.TeamProfile{}, err
	}
	return s.GetTeamProfile(ctx, teamID)
}

func (s *Store) GetTeamProfile(ctx context.Context, teamID string) (domain.TeamProfile, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, team_id, style_metadata, auto_publish_enabled, created_at, updated_at
		from team_profiles
		where team_id = ?`, teamID)
	return scanTeamProfile(row)
}

func (s *Store) UpdateTeamProfile(ctx context.Context, teamID string, input domain.TeamProfile) (domain.TeamProfile, error) {
	metaJSON, err := json.Marshal(input.StyleMetadata)
	if err != nil {
		return domain.TeamProfile{}, fmt.Errorf("marshal style_metadata: %w", err)
	}
	now := nowString()
	_, err = s.db.ExecContext(ctx, `
		update team_profiles
		set style_metadata = ?, auto_publish_enabled = ?, updated_at = ?
		where team_id = ?`,
		string(metaJSON), boolToInt(input.AutoPublishEnabled), now, teamID,
	)
	if err != nil {
		return domain.TeamProfile{}, err
	}
	return s.GetTeamProfile(ctx, teamID)
}

func (s *Store) DeleteTeamProfile(ctx context.Context, teamID string) error {
	_, err := s.db.ExecContext(ctx, `delete from team_profiles where team_id = ?`, teamID)
	return err
}

func scanTeamProfile(scanner rowScanner) (domain.TeamProfile, error) {
	var (
		p         domain.TeamProfile
		metaJSON  string
		autoPubl  int
		createdAt string
		updatedAt string
	)
	if err := scanner.Scan(&p.ID, &p.TeamID, &metaJSON, &autoPubl, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.TeamProfile{}, fmt.Errorf("team profile not found: %w", sql.ErrNoRows)
		}
		return domain.TeamProfile{}, err
	}
	if err := json.Unmarshal([]byte(metaJSON), &p.StyleMetadata); err != nil {
		return domain.TeamProfile{}, fmt.Errorf("unmarshal style_metadata: %w", err)
	}
	p.AutoPublishEnabled = autoPubl != 0
	var err error
	p.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.TeamProfile{}, err
	}
	p.UpdatedAt, err = parseTime(updatedAt)
	return p, err
}

func (s *Store) CreateCampaignFormat(ctx context.Context, teamID string, input domain.CampaignFormat) (domain.CampaignFormat, error) {
	structureJSON := marshalRawMessage(input.Structure, "{}")
	hashtagsJSON, err := marshalStringSlice(input.RequiredHashtags)
	if err != nil {
		return domain.CampaignFormat{}, fmt.Errorf("marshal required_hashtags: %w", err)
	}
	id := uuid.NewString()
	now := nowString()
	_, err = s.db.ExecContext(ctx, `
		insert into campaign_formats (id, team_id, name, weekday, structure, required_hashtags, is_active, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, teamID, input.Name, intPtrToAny(input.Weekday), structureJSON, hashtagsJSON, boolToInt(input.IsActive), now, now,
	)
	if err != nil {
		return domain.CampaignFormat{}, err
	}
	return s.GetCampaignFormatByID(ctx, teamID, id)
}

func (s *Store) ListCampaignFormats(ctx context.Context, teamID string) ([]domain.CampaignFormat, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, name, weekday, structure, required_hashtags, is_active, created_at, updated_at
		from campaign_formats
		where team_id = ?
		order by created_at asc`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectCampaignFormats(rows)
}

func (s *Store) GetCampaignFormatByID(ctx context.Context, teamID string, id string) (domain.CampaignFormat, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, team_id, name, weekday, structure, required_hashtags, is_active, created_at, updated_at
		from campaign_formats
		where team_id = ? and id = ?`, teamID, id)
	return scanCampaignFormat(row)
}

func (s *Store) UpdateCampaignFormat(ctx context.Context, teamID string, id string, input domain.CampaignFormat) (domain.CampaignFormat, error) {
	structureJSON := marshalRawMessage(input.Structure, "{}")
	hashtagsJSON, err := marshalStringSlice(input.RequiredHashtags)
	if err != nil {
		return domain.CampaignFormat{}, fmt.Errorf("marshal required_hashtags: %w", err)
	}
	now := nowString()
	_, err = s.db.ExecContext(ctx, `
		update campaign_formats
		set name = ?, weekday = ?, structure = ?, required_hashtags = ?, is_active = ?, updated_at = ?
		where team_id = ? and id = ?`,
		input.Name, intPtrToAny(input.Weekday), structureJSON, hashtagsJSON, boolToInt(input.IsActive), now, teamID, id,
	)
	if err != nil {
		return domain.CampaignFormat{}, err
	}
	return s.GetCampaignFormatByID(ctx, teamID, id)
}

func (s *Store) DeleteCampaignFormat(ctx context.Context, teamID string, id string) error {
	_, err := s.db.ExecContext(ctx, `delete from campaign_formats where team_id = ? and id = ?`, teamID, id)
	return err
}

func scanCampaignFormat(scanner rowScanner) (domain.CampaignFormat, error) {
	var (
		cf           domain.CampaignFormat
		weekday      sql.NullInt64
		structureRaw string
		hashtagsRaw  string
		isActive     int
		createdAt    string
		updatedAt    string
	)
	if err := scanner.Scan(&cf.ID, &cf.TeamID, &cf.Name, &weekday, &structureRaw, &hashtagsRaw, &isActive, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.CampaignFormat{}, fmt.Errorf("campaign format not found: %w", sql.ErrNoRows)
		}
		return domain.CampaignFormat{}, err
	}
	if weekday.Valid {
		v := int(weekday.Int64)
		cf.Weekday = &v
	}
	cf.Structure = json.RawMessage(structureRaw)
	if err := json.Unmarshal([]byte(hashtagsRaw), &cf.RequiredHashtags); err != nil {
		return domain.CampaignFormat{}, fmt.Errorf("unmarshal required_hashtags: %w", err)
	}
	if cf.RequiredHashtags == nil {
		cf.RequiredHashtags = []string{}
	}
	cf.IsActive = isActive != 0
	var err error
	cf.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.CampaignFormat{}, err
	}
	cf.UpdatedAt, err = parseTime(updatedAt)
	return cf, err
}

func collectCampaignFormats(rows *sql.Rows) ([]domain.CampaignFormat, error) {
	var out []domain.CampaignFormat
	for rows.Next() {
		cf, err := scanCampaignFormat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cf)
	}
	return out, rows.Err()
}

func (s *Store) CreateStyleExample(ctx context.Context, teamID string, input domain.StyleExample) (domain.StyleExample, error) {
	id := uuid.NewString()
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		insert into style_examples (id, team_id, platform, content, notes, created_at)
		values (?, ?, ?, ?, ?, ?)`,
		id, teamID, input.Platform, input.Content, input.Notes, now,
	)
	if err != nil {
		return domain.StyleExample{}, err
	}
	var e domain.StyleExample
	var createdAt string
	if err := s.db.QueryRowContext(ctx, `
		select id, team_id, platform, content, notes, created_at
		from style_examples
		where team_id = ? and id = ?`, teamID, id,
	).Scan(&e.ID, &e.TeamID, &e.Platform, &e.Content, &e.Notes, &createdAt); err != nil {
		return domain.StyleExample{}, err
	}
	e.CreatedAt, err = parseTime(createdAt)
	return e, err
}

func (s *Store) ListStyleExamples(ctx context.Context, teamID string) ([]domain.StyleExample, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, platform, content, notes, created_at
		from style_examples
		where team_id = ?
		order by created_at asc`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StyleExample
	for rows.Next() {
		var (
			e         domain.StyleExample
			createdAt string
		)
		if err := rows.Scan(&e.ID, &e.TeamID, &e.Platform, &e.Content, &e.Notes, &createdAt); err != nil {
			return nil, err
		}
		t, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		e.CreatedAt = t
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) DeleteStyleExample(ctx context.Context, teamID string, id string) error {
	_, err := s.db.ExecContext(ctx, `delete from style_examples where team_id = ? and id = ?`, teamID, id)
	return err
}

func (s *Store) CreateKnowledgeSource(ctx context.Context, teamID string, input domain.KnowledgeSource) (domain.KnowledgeSource, error) {
	id := uuid.NewString()
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		insert into knowledge_sources (id, team_id, source_type, name, content, source_url, media_id, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, teamID, string(input.Type), input.Name, input.Content, input.SourceURL, input.MediaID, now, now,
	)
	if err != nil {
		return domain.KnowledgeSource{}, err
	}
	return s.GetKnowledgeSourceByID(ctx, teamID, id)
}

func (s *Store) ListKnowledgeSources(ctx context.Context, teamID string) ([]domain.KnowledgeSource, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, source_type, name, content, source_url, media_id, created_at, updated_at
		from knowledge_sources
		where team_id = ?
		order by created_at asc`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectKnowledgeSources(rows)
}

func (s *Store) GetKnowledgeSourceByID(ctx context.Context, teamID string, id string) (domain.KnowledgeSource, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, team_id, source_type, name, content, source_url, media_id, created_at, updated_at
		from knowledge_sources
		where team_id = ? and id = ?`, teamID, id)
	return scanKnowledgeSource(row)
}

func (s *Store) UpdateKnowledgeSource(ctx context.Context, teamID string, id string, input domain.KnowledgeSource) (domain.KnowledgeSource, error) {
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		update knowledge_sources
		set source_type = ?, name = ?, content = ?, source_url = ?, media_id = ?, updated_at = ?
		where team_id = ? and id = ?`,
		string(input.Type), input.Name, input.Content, input.SourceURL, input.MediaID, now, teamID, id,
	)
	if err != nil {
		return domain.KnowledgeSource{}, err
	}
	return s.GetKnowledgeSourceByID(ctx, teamID, id)
}

func (s *Store) DeleteKnowledgeSource(ctx context.Context, teamID string, id string) error {
	_, err := s.db.ExecContext(ctx, `delete from knowledge_sources where team_id = ? and id = ?`, teamID, id)
	return err
}

func scanKnowledgeSource(scanner rowScanner) (domain.KnowledgeSource, error) {
	var (
		ks          domain.KnowledgeSource
		sourceType  string
		createdAt   string
		updatedAt   string
	)
	if err := scanner.Scan(&ks.ID, &ks.TeamID, &sourceType, &ks.Name, &ks.Content, &ks.SourceURL, &ks.MediaID, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.KnowledgeSource{}, fmt.Errorf("knowledge source not found: %w", sql.ErrNoRows)
		}
		return domain.KnowledgeSource{}, err
	}
	ks.Type = domain.KnowledgeSourceType(sourceType)
	var err error
	ks.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.KnowledgeSource{}, err
	}
	ks.UpdatedAt, err = parseTime(updatedAt)
	return ks, err
}

func collectKnowledgeSources(rows *sql.Rows) ([]domain.KnowledgeSource, error) {
	var out []domain.KnowledgeSource
	for rows.Next() {
		ks, err := scanKnowledgeSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ks)
	}
	return out, rows.Err()
}

func (s *Store) CreateAIJob(ctx context.Context, input domain.AIJob) (domain.AIJob, error) {
	payloadJSON := marshalRawMessage(input.Payload, "{}")
	id := input.ID
	if id == "" {
		id = uuid.NewString()
	}
	status := input.Status
	if status == "" {
		status = domain.AIJobStatusPending
	}
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		insert into ai_jobs (id, team_id, author_user_id, job_type, status, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.TeamID, input.AuthorUserID, string(input.Type), string(status), payloadJSON, now, now,
	)
	if err != nil {
		return domain.AIJob{}, err
	}
	return s.GetAIJobByID(ctx, input.TeamID, id)
}

func (s *Store) GetAIJobByID(ctx context.Context, teamID string, id string) (domain.AIJob, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, team_id, author_user_id, job_type, status, payload, result, error_message, created_at, updated_at, completed_at
		from ai_jobs
		where team_id = ? and id = ?`, teamID, id)
	return scanAIJob(row)
}

func (s *Store) GetAIJobByIDGlobal(ctx context.Context, id string) (domain.AIJob, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, team_id, author_user_id, job_type, status, payload, result, error_message, created_at, updated_at, completed_at
		from ai_jobs
		where id = ?`, id)
	return scanAIJob(row)
}

func (s *Store) ListAIJobs(ctx context.Context, teamID string, limit int) ([]domain.AIJob, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, job_type, status, payload, result, error_message, created_at, updated_at, completed_at
		from ai_jobs
		where team_id = ?
		order by created_at desc
		limit ?`, teamID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectAIJobs(rows)
}

func (s *Store) UpdateAIJobStatus(ctx context.Context, id string, status domain.AIJobStatus, result []byte, errorMsg string) error {
	now := nowString()
	var completedAt any
	if status == domain.AIJobStatusCompleted || status == domain.AIJobStatusFailed {
		completedAt = now
	}
	var resultVal any
	if len(result) > 0 {
		resultVal = string(result)
	}
	_, err := s.db.ExecContext(ctx, `
		update ai_jobs
		set status = ?, result = ?, error_message = ?, updated_at = ?, completed_at = ?
		where id = ?`,
		string(status), resultVal, nullableString(errorMsg), now, completedAt, id,
	)
	return err
}

func (s *Store) ListPendingAIJobs(ctx context.Context, limit int) ([]domain.AIJob, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, job_type, status, payload, result, error_message, created_at, updated_at, completed_at
		from ai_jobs
		where status = ?
		order by created_at asc
		limit ?`, string(domain.AIJobStatusPending), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectAIJobs(rows)
}

func scanAIJob(scanner rowScanner) (domain.AIJob, error) {
	var (
		job         domain.AIJob
		payloadRaw  string
		resultRaw   sql.NullString
		errMsg      sql.NullString
		completedAt sql.NullString
		createdAt   string
		updatedAt   string
	)
	if err := scanner.Scan(
		&job.ID, &job.TeamID, &job.AuthorUserID, &job.Type, &job.Status,
		&payloadRaw, &resultRaw, &errMsg,
		&createdAt, &updatedAt, &completedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AIJob{}, fmt.Errorf("ai job not found: %w", sql.ErrNoRows)
		}
		return domain.AIJob{}, err
	}
	if strings.TrimSpace(payloadRaw) != "" {
		job.Payload = json.RawMessage(payloadRaw)
	}
	if resultRaw.Valid && strings.TrimSpace(resultRaw.String) != "" {
		job.Result = json.RawMessage(resultRaw.String)
	}
	job.ErrorMessage = errMsg.String
	var err error
	job.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.AIJob{}, err
	}
	job.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.AIJob{}, err
	}
	if completedAt.Valid && strings.TrimSpace(completedAt.String) != "" {
		t, err := parseTime(completedAt.String)
		if err != nil {
			return domain.AIJob{}, err
		}
		job.CompletedAt = &t
	}
	return job, nil
}

func collectAIJobs(rows *sql.Rows) ([]domain.AIJob, error) {
	var out []domain.AIJob
	for rows.Next() {
		job, err := scanAIJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (s *Store) GetAIServiceConfig(ctx context.Context, teamID string) (domain.AIServiceConfig, error) {
	var (
		cfg              domain.AIServiceConfig
		teamIDNull       sql.NullString
		apiKeyCiphertext string
		createdAt        string
	)
	err := s.db.QueryRowContext(ctx, `
		select id, team_id, provider, model, base_url, api_key_ciphertext, description, created_at
		from ai_service_configs
		where team_id = ?`, teamID,
	).Scan(&cfg.ID, &teamIDNull, &cfg.Provider, &cfg.Model, &cfg.BaseURL, &apiKeyCiphertext, &cfg.Description, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AIServiceConfig{}, fmt.Errorf("ai service config not found: %w", sql.ErrNoRows)
		}
		return domain.AIServiceConfig{}, err
	}
	if teamIDNull.Valid && teamIDNull.String != "" {
		tid := teamIDNull.String
		cfg.TeamID = &tid
	}
	if apiKeyCiphertext != "" {
		apiKey, decErr := s.encrypter.Decrypt(apiKeyCiphertext)
		if decErr != nil {
			return domain.AIServiceConfig{}, fmt.Errorf("decrypt ai api key: %w", decErr)
		}
		cfg.APIKey = apiKey
		cfg.APIKeySet = true
	}
	cfg.CreatedAt, err = parseTime(createdAt)
	return cfg, err
}

func (s *Store) UpsertAIServiceConfig(ctx context.Context, teamID string, input domain.AIServiceConfig) (domain.AIServiceConfig, error) {
	now := nowString()
	apiKeyCiphertext := ""
	if strings.TrimSpace(input.APIKey) != "" {
		var encErr error
		apiKeyCiphertext, encErr = s.encrypter.Encrypt(input.APIKey)
		if encErr != nil {
			return domain.AIServiceConfig{}, fmt.Errorf("encrypt ai api key: %w", encErr)
		}
	}

	var existingID string
	err := s.db.QueryRowContext(ctx, `select id from ai_service_configs where team_id = ?`, teamID).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.AIServiceConfig{}, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		newID := uuid.NewString()
		_, err = s.db.ExecContext(ctx, `
			insert into ai_service_configs (id, team_id, provider, model, base_url, api_key_ciphertext, description, created_at)
			values (?, ?, ?, ?, ?, ?, ?, ?)`,
			newID, teamID, input.Provider, input.Model, input.BaseURL, apiKeyCiphertext, input.Description, now,
		)
	} else if apiKeyCiphertext != "" {
		_, err = s.db.ExecContext(ctx, `
			update ai_service_configs
			set provider = ?, model = ?, base_url = ?, api_key_ciphertext = ?, description = ?
			where team_id = ?`,
			input.Provider, input.Model, input.BaseURL, apiKeyCiphertext, input.Description, teamID,
		)
	} else {
		// Empty API key on update keeps the stored key.
		_, err = s.db.ExecContext(ctx, `
			update ai_service_configs
			set provider = ?, model = ?, base_url = ?, description = ?
			where team_id = ?`,
			input.Provider, input.Model, input.BaseURL, input.Description, teamID,
		)
	}
	if err != nil {
		return domain.AIServiceConfig{}, err
	}
	return s.GetAIServiceConfig(ctx, teamID)
}

func (s *Store) CreateRSSFeedConfig(ctx context.Context, teamID string, input domain.RSSFeedConfig) (domain.RSSFeedConfig, error) {
	id := uuid.NewString()
	now := nowString()
	targetJSON, err := encodeMediaIDsJSON(domain.NormalizeMediaIDs(input.TargetAccountIDs))
	if err != nil {
		return domain.RSSFeedConfig{}, err
	}
	syncMode := string(domain.NormalizeRSSInitialSyncMode(string(input.InitialSyncMode)))
	contentTemplate := input.NormalizedContentTemplate()
	outputMode := string(domain.NormalizeAutomationOutputMode(string(input.OutputMode)))
	maxPosts := input.NormalizedMaxPostsPerDay()
	_, err = s.db.ExecContext(ctx, `
		insert into rss_feed_configs (
			id, team_id, feed_url, name, is_active, ai_enhance_enabled, content_template, title_template, title_hint, output_mode, max_posts_per_day,
			prompt_hint, target_account_ids, tonality, initial_sync_mode, created_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, teamID, input.FeedURL, input.Name, boolToInt(input.IsActive), boolToInt(input.AiEnhanceEnabled), contentTemplate, input.NormalizedTitleTemplate(), input.TitleHint, outputMode, maxPosts,
		input.PromptHint, targetJSON, input.Tonality, syncMode, now,
	)
	if err != nil {
		return domain.RSSFeedConfig{}, err
	}
	row := s.db.QueryRowContext(ctx, rssFeedSelectQuery+` where team_id = ? and id = ?`, teamID, id)
	return scanRSSFeedConfig(row)
}

func (s *Store) GetRSSFeedConfigByID(ctx context.Context, teamID string, id string) (domain.RSSFeedConfig, error) {
	row := s.db.QueryRowContext(ctx, rssFeedSelectQuery+` where team_id = ? and id = ?`, teamID, id)
	return scanRSSFeedConfig(row)
}

func (s *Store) ListRSSFeedConfigs(ctx context.Context, teamID string) ([]domain.RSSFeedConfig, error) {
	rows, err := s.db.QueryContext(ctx, rssFeedSelectQuery+`
		where team_id = ?
		order by created_at asc`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RSSFeedConfig
	for rows.Next() {
		cfg, err := scanRSSFeedConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (s *Store) UpdateRSSFeedConfig(ctx context.Context, teamID string, id string, input domain.RSSFeedConfig) (domain.RSSFeedConfig, error) {
	targetJSON, err := encodeMediaIDsJSON(domain.NormalizeMediaIDs(input.TargetAccountIDs))
	if err != nil {
		return domain.RSSFeedConfig{}, err
	}
	var lastFetched any
	if input.LastFetchedAt != nil {
		lastFetched = formatTime(*input.LastFetchedAt)
	}
	syncMode := string(domain.NormalizeRSSInitialSyncMode(string(input.InitialSyncMode)))
	contentTemplate := input.NormalizedContentTemplate()
	outputMode := string(domain.NormalizeAutomationOutputMode(string(input.OutputMode)))
	maxPosts := input.NormalizedMaxPostsPerDay()
	_, err = s.db.ExecContext(ctx, `
		update rss_feed_configs
		set feed_url = ?, name = ?, is_active = ?, ai_enhance_enabled = ?, content_template = ?, title_template = ?, title_hint = ?, output_mode = ?, max_posts_per_day = ?,
		    prompt_hint = ?, target_account_ids = ?, tonality = ?,
		    initial_sync_mode = ?, last_fetched_at = coalesce(?, last_fetched_at)
		where team_id = ? and id = ?`,
		input.FeedURL, input.Name, boolToInt(input.IsActive), boolToInt(input.AiEnhanceEnabled), contentTemplate, input.NormalizedTitleTemplate(), input.TitleHint, outputMode, maxPosts,
		input.PromptHint, targetJSON, input.Tonality, syncMode, lastFetched, teamID, id,
	)
	if err != nil {
		return domain.RSSFeedConfig{}, err
	}
	row := s.db.QueryRowContext(ctx, rssFeedSelectQuery+` where team_id = ? and id = ?`, teamID, id)
	return scanRSSFeedConfig(row)
}

const rssFeedSelectQuery = `
		select id, team_id, feed_url, name, is_active, ai_enhance_enabled, content_template, title_template, title_hint, output_mode, max_posts_per_day, counter_next,
		       prompt_hint, target_account_ids, tonality, initial_sync_mode, last_fetched_at, created_at
		from rss_feed_configs
`

func (s *Store) DeleteRSSFeedConfig(ctx context.Context, teamID string, id string) error {
	_, err := s.db.ExecContext(ctx, `delete from rss_feed_configs where team_id = ? and id = ?`, teamID, id)
	return err
}

func scanRSSFeedConfig(scanner rowScanner) (domain.RSSFeedConfig, error) {
	var (
		cfg               domain.RSSFeedConfig
		isActive          int
		aiEnhanceEnabled  int
		targetRaw         string
		lastFetchedAt     sql.NullString
		createdAt         string
		outputMode        string
	)
	var syncMode string
	if err := scanner.Scan(
		&cfg.ID, &cfg.TeamID, &cfg.FeedURL, &cfg.Name, &isActive, &aiEnhanceEnabled, &cfg.ContentTemplate, &cfg.TitleTemplate, &cfg.TitleHint, &outputMode, &cfg.MaxPostsPerDay, &cfg.CounterNext,
		&cfg.PromptHint, &targetRaw, &cfg.Tonality, &syncMode, &lastFetchedAt, &createdAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RSSFeedConfig{}, fmt.Errorf("rss feed config not found: %w", sql.ErrNoRows)
		}
		return domain.RSSFeedConfig{}, err
	}
	cfg.IsActive = isActive != 0
	cfg.AiEnhanceEnabled = aiEnhanceEnabled != 0
	cfg.OutputMode = domain.NormalizeAutomationOutputMode(outputMode)
	cfg.InitialSyncMode = domain.NormalizeRSSInitialSyncMode(syncMode)
	if err := json.Unmarshal([]byte(targetRaw), &cfg.TargetAccountIDs); err != nil {
		return domain.RSSFeedConfig{}, err
	}
	if lastFetchedAt.Valid && strings.TrimSpace(lastFetchedAt.String) != "" {
		t, err := parseTime(lastFetchedAt.String)
		if err != nil {
			return domain.RSSFeedConfig{}, err
		}
		cfg.LastFetchedAt = &t
	}
	var err error
	cfg.CreatedAt, err = parseTime(createdAt)
	return cfg, err
}

func (s *Store) GetProactiveTriggerSettings(ctx context.Context, teamID string) (domain.ProactiveTriggerSettings, error) {
	var (
		p         domain.ProactiveTriggerSettings
		autoFill  int
		createdAt string
		updatedAt string
	)
	err := s.db.QueryRowContext(ctx, `
		select id, team_id, content_gap_threshold_days, auto_fill_enabled, max_triggers_per_day, cron_schedule, created_at, updated_at
		from proactive_trigger_settings
		where team_id = ?`, teamID,
	).Scan(&p.ID, &p.TeamID, &p.ContentGapThresholdDays, &autoFill, &p.MaxTriggersPerDay, &p.CronSchedule, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ProactiveTriggerSettings{}, fmt.Errorf("proactive trigger settings not found: %w", sql.ErrNoRows)
		}
		return domain.ProactiveTriggerSettings{}, err
	}
	p.AutoFillEnabled = autoFill != 0
	p.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.ProactiveTriggerSettings{}, err
	}
	p.UpdatedAt, err = parseTime(updatedAt)
	return p, err
}

func (s *Store) UpsertProactiveTriggerSettings(ctx context.Context, teamID string, input domain.ProactiveTriggerSettings) (domain.ProactiveTriggerSettings, error) {
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		insert into proactive_trigger_settings (id, team_id, content_gap_threshold_days, auto_fill_enabled, max_triggers_per_day, cron_schedule, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(team_id) do update set
			content_gap_threshold_days = excluded.content_gap_threshold_days,
			auto_fill_enabled = excluded.auto_fill_enabled,
			max_triggers_per_day = excluded.max_triggers_per_day,
			cron_schedule = excluded.cron_schedule,
			updated_at = excluded.updated_at`,
		uuid.NewString(), teamID,
		input.ContentGapThresholdDays, boolToInt(input.AutoFillEnabled),
		input.MaxTriggersPerDay, input.CronSchedule,
		now, now,
	)
	if err != nil {
		return domain.ProactiveTriggerSettings{}, err
	}
	return s.GetProactiveTriggerSettings(ctx, teamID)
}

func (s *Store) GetTeamAIContext(ctx context.Context, teamID string) (domain.AIContext, error) {
	team, err := s.GetTeamByID(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, err
	}

	var aiCtxProfile *domain.TeamProfile
	profile, profileErr := s.GetTeamProfile(ctx, teamID)
	if profileErr != nil {
		if !errors.Is(profileErr, sql.ErrNoRows) {
			return domain.AIContext{}, profileErr
		}
	} else {
		aiCtxProfile = &profile
	}

	formats, err := s.ListCampaignFormats(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, err
	}

	examples, err := s.ListStyleExamples(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, err
	}

	knowledgeSources, err := s.ListKnowledgeSources(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, err
	}

	recentRows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, title, content, scheduled_at, status, source,
		       attempt_count, last_error, visibility, media_ids, media_exclude_by_account,
		       post_template_id, template_counter, created_at, updated_at
		from scheduled_posts
		where team_id = ? and status = 'posted' and trim(content) != ''
		order by scheduled_at desc
		limit ?`, teamID, domain.AIContextRecentPostsLimit)
	if err != nil {
		return domain.AIContext{}, err
	}
	defer recentRows.Close()
	recentPosts, err := collectPosts(recentRows)
	if err != nil {
		return domain.AIContext{}, err
	}

	upcomingRows, err := s.db.QueryContext(ctx, `
		select id, team_id, author_user_id, title, content, scheduled_at, status, source,
		       attempt_count, last_error, visibility, media_ids, media_exclude_by_account,
		       post_template_id, template_counter, created_at, updated_at
		from scheduled_posts
		where team_id = ? and status in ('pending', 'draft') and scheduled_at >= ?
		order by scheduled_at asc
		limit ?`, teamID, nowString(), domain.AIContextUpcomingPostsLimit)
	if err != nil {
		return domain.AIContext{}, err
	}
	defer upcomingRows.Close()
	upcomingPosts, err := collectPosts(upcomingRows)
	if err != nil {
		return domain.AIContext{}, err
	}

	accounts, err := s.ListTeamAccounts(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, err
	}
	accountSummaries := make([]domain.AIAccountSummary, 0, len(accounts))
	for _, acc := range accounts {
		accountSummaries = append(accountSummaries, domain.AIAccountSummary{
			ID:       acc.ID,
			Provider: acc.Provider,
			Username: acc.Username,
			MaxChars: domain.MaxCharsForProvider(acc.Provider, acc.MaxCharsOverride),
		})
	}

	engagementHours, err := s.GetTeamEngagementHourHistogram(ctx, teamID, 90)
	if err != nil {
		return domain.AIContext{}, err
	}

	topHashtags, err := s.ListTeamHashtagPerformance(ctx, teamID, 90, "", 20)
	if err != nil {
		return domain.AIContext{}, err
	}

	return domain.AIContext{
		Team:             team,
		Profile:          aiCtxProfile,
		CampaignFormats:  formats,
		StyleExamples:    examples,
		KnowledgeSources: knowledgeSources,
		RecentPosts:      recentPosts,
		Accounts:         accountSummaries,
		UpcomingPosts:    upcomingPosts,
		EngagementHours:  engagementHours,
		TopHashtags:      topHashtags,
	}, nil
}

func (s *Store) ListAIEnabledTeams(ctx context.Context) ([]domain.Team, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, name, description, created_at, is_ai_enabled, scheduling_prefs, brand_color
		from teams
		where is_ai_enabled = 1
		order by name asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []domain.Team
	for rows.Next() {
		var (
			team            domain.Team
			isAIEnabled     int
			schedulingPrefs sql.NullString
			brandColor      sql.NullString
			createdAt       string
		)
		if err := rows.Scan(
			&team.ID, &team.Name, &team.Description, &createdAt,
			&isAIEnabled, &schedulingPrefs, &brandColor,
		); err != nil {
			return nil, err
		}
		team.IsAIEnabled = isAIEnabled != 0
		team.BrandColor = brandColor.String
		if schedulingPrefs.Valid && strings.TrimSpace(schedulingPrefs.String) != "" {
			prefs, err := domain.ParseTeamSchedulingPrefsJSON(schedulingPrefs.String)
			if err != nil {
				return nil, err
			}
			team.SchedulingPrefs = prefs
		} else {
			team.SchedulingPrefs = domain.DefaultTeamSchedulingPreferences()
		}
		t, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		team.CreatedAt = t
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

func marshalRawMessage(raw json.RawMessage, def string) string {
	if len(raw) == 0 {
		return def
	}
	return string(raw)
}

func marshalStringSlice(s []string) (string, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func intPtrToAny(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
