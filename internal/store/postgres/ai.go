package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateTeamProfile(ctx context.Context, teamID string, input domain.TeamProfile) (domain.TeamProfile, error) {
	metaJSON, err := json.Marshal(input.StyleMetadata)
	if err != nil {
		return domain.TeamProfile{}, fmt.Errorf("CreateTeamProfile: marshal style_metadata: %w", err)
	}
	const query = `
		INSERT INTO team_profiles (team_id, style_metadata, auto_publish_enabled)
		VALUES ($1, $2, $3)
		RETURNING id, team_id, style_metadata, auto_publish_enabled, created_at, updated_at
	`
	profile, err := scanTeamProfile(s.pool.QueryRow(ctx, query, teamID, metaJSON, input.AutoPublishEnabled))
	if err != nil {
		return domain.TeamProfile{}, fmt.Errorf("CreateTeamProfile: %w", err)
	}
	return profile, nil
}

func (s *Store) GetTeamProfile(ctx context.Context, teamID string) (domain.TeamProfile, error) {
	const query = `
		SELECT id, team_id, style_metadata, auto_publish_enabled, created_at, updated_at
		FROM team_profiles
		WHERE team_id = $1
	`
	profile, err := scanTeamProfile(s.pool.QueryRow(ctx, query, teamID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TeamProfile{}, fmt.Errorf("team profile not found: %w", pgx.ErrNoRows)
		}
		return domain.TeamProfile{}, fmt.Errorf("GetTeamProfile: %w", err)
	}
	return profile, nil
}

func (s *Store) UpdateTeamProfile(ctx context.Context, teamID string, input domain.TeamProfile) (domain.TeamProfile, error) {
	metaJSON, err := json.Marshal(input.StyleMetadata)
	if err != nil {
		return domain.TeamProfile{}, fmt.Errorf("UpdateTeamProfile: marshal style_metadata: %w", err)
	}
	const query = `
		UPDATE team_profiles
		SET style_metadata = $2, auto_publish_enabled = $3, updated_at = now()
		WHERE team_id = $1
		RETURNING id, team_id, style_metadata, auto_publish_enabled, created_at, updated_at
	`
	profile, err := scanTeamProfile(s.pool.QueryRow(ctx, query, teamID, metaJSON, input.AutoPublishEnabled))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TeamProfile{}, fmt.Errorf("team profile not found: %w", pgx.ErrNoRows)
		}
		return domain.TeamProfile{}, fmt.Errorf("UpdateTeamProfile: %w", err)
	}
	return profile, nil
}

func (s *Store) DeleteTeamProfile(ctx context.Context, teamID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM team_profiles WHERE team_id = $1`, teamID)
	if err != nil {
		return fmt.Errorf("DeleteTeamProfile: %w", err)
	}
	return nil
}

func scanTeamProfile(row interface{ Scan(dest ...any) error }) (domain.TeamProfile, error) {
	var profile domain.TeamProfile
	var metaRaw []byte
	if err := row.Scan(
		&profile.ID,
		&profile.TeamID,
		&metaRaw,
		&profile.AutoPublishEnabled,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	); err != nil {
		return domain.TeamProfile{}, err
	}
	if len(metaRaw) > 0 {
		if err := json.Unmarshal(metaRaw, &profile.StyleMetadata); err != nil {
			return domain.TeamProfile{}, fmt.Errorf("unmarshal style_metadata: %w", err)
		}
	}
	return profile, nil
}

func (s *Store) CreateCampaignFormat(ctx context.Context, teamID string, input domain.CampaignFormat) (domain.CampaignFormat, error) {
	const query = `
		INSERT INTO campaign_formats (team_id, name, weekday, structure, required_hashtags, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, team_id, name, weekday, structure, required_hashtags, is_active, created_at, updated_at
	`
	hashtags := input.RequiredHashtags
	if hashtags == nil {
		hashtags = []string{}
	}
	cf, err := scanCampaignFormat(s.pool.QueryRow(ctx, query,
		teamID, input.Name, input.Weekday, input.Structure, hashtags, input.IsActive,
	))
	if err != nil {
		return domain.CampaignFormat{}, fmt.Errorf("CreateCampaignFormat: %w", err)
	}
	return cf, nil
}

func (s *Store) ListCampaignFormats(ctx context.Context, teamID string) ([]domain.CampaignFormat, error) {
	const query = `
		SELECT id, team_id, name, weekday, structure, required_hashtags, is_active, created_at, updated_at
		FROM campaign_formats
		WHERE team_id = $1
		ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("ListCampaignFormats: %w", err)
	}
	defer rows.Close()

	var formats []domain.CampaignFormat
	for rows.Next() {
		cf, err := scanCampaignFormat(rows)
		if err != nil {
			return nil, fmt.Errorf("ListCampaignFormats: scan: %w", err)
		}
		formats = append(formats, cf)
	}
	return formats, rows.Err()
}

func (s *Store) GetCampaignFormatByID(ctx context.Context, teamID string, id string) (domain.CampaignFormat, error) {
	const query = `
		SELECT id, team_id, name, weekday, structure, required_hashtags, is_active, created_at, updated_at
		FROM campaign_formats
		WHERE team_id = $1 AND id = $2
	`
	cf, err := scanCampaignFormat(s.pool.QueryRow(ctx, query, teamID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CampaignFormat{}, fmt.Errorf("campaign format not found: %w", pgx.ErrNoRows)
		}
		return domain.CampaignFormat{}, fmt.Errorf("GetCampaignFormatByID: %w", err)
	}
	return cf, nil
}

func (s *Store) UpdateCampaignFormat(ctx context.Context, teamID string, id string, input domain.CampaignFormat) (domain.CampaignFormat, error) {
	const query = `
		UPDATE campaign_formats
		SET name = $3, weekday = $4, structure = $5, required_hashtags = $6, is_active = $7, updated_at = now()
		WHERE team_id = $1 AND id = $2
		RETURNING id, team_id, name, weekday, structure, required_hashtags, is_active, created_at, updated_at
	`
	hashtags := input.RequiredHashtags
	if hashtags == nil {
		hashtags = []string{}
	}
	cf, err := scanCampaignFormat(s.pool.QueryRow(ctx, query,
		teamID, id, input.Name, input.Weekday, input.Structure, hashtags, input.IsActive,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CampaignFormat{}, fmt.Errorf("campaign format not found: %w", pgx.ErrNoRows)
		}
		return domain.CampaignFormat{}, fmt.Errorf("UpdateCampaignFormat: %w", err)
	}
	return cf, nil
}

func (s *Store) DeleteCampaignFormat(ctx context.Context, teamID string, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM campaign_formats WHERE team_id = $1 AND id = $2`, teamID, id)
	if err != nil {
		return fmt.Errorf("DeleteCampaignFormat: %w", err)
	}
	return nil
}

func scanCampaignFormat(row interface{ Scan(dest ...any) error }) (domain.CampaignFormat, error) {
	var cf domain.CampaignFormat
	var weekday sql.NullInt32
	var structRaw []byte
	if err := row.Scan(
		&cf.ID,
		&cf.TeamID,
		&cf.Name,
		&weekday,
		&structRaw,
		&cf.RequiredHashtags,
		&cf.IsActive,
		&cf.CreatedAt,
		&cf.UpdatedAt,
	); err != nil {
		return domain.CampaignFormat{}, err
	}
	if weekday.Valid {
		v := int(weekday.Int32)
		cf.Weekday = &v
	}
	cf.Structure = json.RawMessage(structRaw)
	if cf.RequiredHashtags == nil {
		cf.RequiredHashtags = []string{}
	}
	return cf, nil
}

func (s *Store) CreateStyleExample(ctx context.Context, teamID string, input domain.StyleExample) (domain.StyleExample, error) {
	const query = `
		INSERT INTO style_examples (team_id, platform, content, notes)
		VALUES ($1, $2, $3, $4)
		RETURNING id, team_id, platform, content, notes, created_at
	`
	ex, err := scanStyleExample(s.pool.QueryRow(ctx, query,
		teamID, input.Platform, input.Content, input.Notes,
	))
	if err != nil {
		return domain.StyleExample{}, fmt.Errorf("CreateStyleExample: %w", err)
	}
	return ex, nil
}

func (s *Store) ListStyleExamples(ctx context.Context, teamID string) ([]domain.StyleExample, error) {
	const query = `
		SELECT id, team_id, platform, content, notes, created_at
		FROM style_examples
		WHERE team_id = $1
		ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("ListStyleExamples: %w", err)
	}
	defer rows.Close()

	var examples []domain.StyleExample
	for rows.Next() {
		ex, err := scanStyleExample(rows)
		if err != nil {
			return nil, fmt.Errorf("ListStyleExamples: scan: %w", err)
		}
		examples = append(examples, ex)
	}
	return examples, rows.Err()
}

func (s *Store) DeleteStyleExample(ctx context.Context, teamID string, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM style_examples WHERE team_id = $1 AND id = $2`, teamID, id)
	if err != nil {
		return fmt.Errorf("DeleteStyleExample: %w", err)
	}
	return nil
}

func (s *Store) CreateKnowledgeSource(ctx context.Context, teamID string, input domain.KnowledgeSource) (domain.KnowledgeSource, error) {
	const query = `
		INSERT INTO knowledge_sources (team_id, source_type, name, content, source_url, media_id)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))
		RETURNING id, team_id, source_type, name, content, coalesce(source_url, ''), coalesce(media_id::text, ''), created_at, updated_at
	`
	ks, err := scanKnowledgeSource(s.pool.QueryRow(ctx, query,
		teamID, string(input.Type), input.Name, input.Content, input.SourceURL, input.MediaID,
	))
	if err != nil {
		return domain.KnowledgeSource{}, fmt.Errorf("CreateKnowledgeSource: %w", err)
	}
	return ks, nil
}

func (s *Store) ListKnowledgeSources(ctx context.Context, teamID string) ([]domain.KnowledgeSource, error) {
	const query = `
		SELECT id, team_id, source_type, name, content, coalesce(source_url, ''), coalesce(media_id::text, ''), created_at, updated_at
		FROM knowledge_sources
		WHERE team_id = $1
		ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("ListKnowledgeSources: %w", err)
	}
	defer rows.Close()

	var sources []domain.KnowledgeSource
	for rows.Next() {
		ks, err := scanKnowledgeSource(rows)
		if err != nil {
			return nil, fmt.Errorf("ListKnowledgeSources: scan: %w", err)
		}
		sources = append(sources, ks)
	}
	return sources, rows.Err()
}

func (s *Store) GetKnowledgeSourceByID(ctx context.Context, teamID string, id string) (domain.KnowledgeSource, error) {
	const query = `
		SELECT id, team_id, source_type, name, content, coalesce(source_url, ''), coalesce(media_id::text, ''), created_at, updated_at
		FROM knowledge_sources
		WHERE team_id = $1 AND id = $2
	`
	ks, err := scanKnowledgeSource(s.pool.QueryRow(ctx, query, teamID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.KnowledgeSource{}, fmt.Errorf("knowledge source not found: %w", pgx.ErrNoRows)
		}
		return domain.KnowledgeSource{}, fmt.Errorf("GetKnowledgeSourceByID: %w", err)
	}
	return ks, nil
}

func (s *Store) UpdateKnowledgeSource(ctx context.Context, teamID string, id string, input domain.KnowledgeSource) (domain.KnowledgeSource, error) {
	const query = `
		UPDATE knowledge_sources
		SET source_type = $3, name = $4, content = $5, source_url = $6, media_id = NULLIF($7, ''), updated_at = now()
		WHERE team_id = $1 AND id = $2
		RETURNING id, team_id, source_type, name, content, coalesce(source_url, ''), coalesce(media_id::text, ''), created_at, updated_at
	`
	ks, err := scanKnowledgeSource(s.pool.QueryRow(ctx, query,
		teamID, id, string(input.Type), input.Name, input.Content, input.SourceURL, input.MediaID,
	))
	if err != nil {
		return domain.KnowledgeSource{}, fmt.Errorf("UpdateKnowledgeSource: %w", err)
	}
	return ks, nil
}

func (s *Store) DeleteKnowledgeSource(ctx context.Context, teamID string, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM knowledge_sources WHERE team_id = $1 AND id = $2`, teamID, id)
	if err != nil {
		return fmt.Errorf("DeleteKnowledgeSource: %w", err)
	}
	return nil
}

func scanKnowledgeSource(row interface{ Scan(dest ...any) error }) (domain.KnowledgeSource, error) {
	var ks domain.KnowledgeSource
	var sourceType string
	if err := row.Scan(
		&ks.ID,
		&ks.TeamID,
		&sourceType,
		&ks.Name,
		&ks.Content,
		&ks.SourceURL,
		&ks.MediaID,
		&ks.CreatedAt,
		&ks.UpdatedAt,
	); err != nil {
		return domain.KnowledgeSource{}, err
	}
	ks.Type = domain.KnowledgeSourceType(sourceType)
	return ks, nil
}

func scanStyleExample(row interface{ Scan(dest ...any) error }) (domain.StyleExample, error) {
	var ex domain.StyleExample
	if err := row.Scan(
		&ex.ID,
		&ex.TeamID,
		&ex.Platform,
		&ex.Content,
		&ex.Notes,
		&ex.CreatedAt,
	); err != nil {
		return domain.StyleExample{}, err
	}
	return ex, nil
}

func (s *Store) CreateAIJob(ctx context.Context, input domain.AIJob) (domain.AIJob, error) {
	status := input.Status
	if status == "" {
		status = domain.AIJobStatusPending
	}
	const query = `
		INSERT INTO ai_jobs (team_id, author_user_id, job_type, status, payload)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, team_id, author_user_id, job_type, status, payload, result, coalesce(error_message, ''), created_at, updated_at, completed_at
	`
	payload := input.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	job, err := scanAIJob(s.pool.QueryRow(ctx, query,
		input.TeamID, input.AuthorUserID, string(input.Type), string(status), payload,
	))
	if err != nil {
		return domain.AIJob{}, fmt.Errorf("CreateAIJob: %w", err)
	}
	return job, nil
}

func (s *Store) GetAIJobByID(ctx context.Context, teamID string, id string) (domain.AIJob, error) {
	const query = `
		SELECT id, team_id, author_user_id, job_type, status, payload, result, coalesce(error_message, ''), created_at, updated_at, completed_at
		FROM ai_jobs
		WHERE team_id = $1 AND id = $2
	`
	job, err := scanAIJob(s.pool.QueryRow(ctx, query, teamID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.AIJob{}, fmt.Errorf("ai job not found: %w", pgx.ErrNoRows)
		}
		return domain.AIJob{}, fmt.Errorf("GetAIJobByID: %w", err)
	}
	return job, nil
}

func (s *Store) GetAIJobByIDGlobal(ctx context.Context, id string) (domain.AIJob, error) {
	const query = `
		SELECT id, team_id, author_user_id, job_type, status, payload, result, coalesce(error_message, ''), created_at, updated_at, completed_at
		FROM ai_jobs
		WHERE id = $1
	`
	job, err := scanAIJob(s.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.AIJob{}, fmt.Errorf("ai job not found: %w", pgx.ErrNoRows)
		}
		return domain.AIJob{}, fmt.Errorf("GetAIJobByIDGlobal: %w", err)
	}
	return job, nil
}

func (s *Store) ListAIJobs(ctx context.Context, teamID string, limit int) ([]domain.AIJob, error) {
	const query = `
		SELECT id, team_id, author_user_id, job_type, status, payload, result, coalesce(error_message, ''), created_at, updated_at, completed_at
		FROM ai_jobs
		WHERE team_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	return s.listAIJobs(ctx, query, teamID, limit)
}

func (s *Store) UpdateAIJobStatus(ctx context.Context, id string, status domain.AIJobStatus, result []byte, errorMsg string) error {
	isTerminal := status == domain.AIJobStatusCompleted || status == domain.AIJobStatusFailed

	var err error
	switch {
	case len(result) > 0 && isTerminal:
		_, err = s.pool.Exec(ctx,
			`UPDATE ai_jobs SET status=$2, result=$3, error_message=NULLIF($4,''), updated_at=now(), completed_at=now() WHERE id=$1`,
			id, string(status), json.RawMessage(result), errorMsg)
	case len(result) > 0:
		_, err = s.pool.Exec(ctx,
			`UPDATE ai_jobs SET status=$2, result=$3, error_message=NULLIF($4,''), updated_at=now() WHERE id=$1`,
			id, string(status), json.RawMessage(result), errorMsg)
	case isTerminal:
		_, err = s.pool.Exec(ctx,
			`UPDATE ai_jobs SET status=$2, error_message=NULLIF($3,''), updated_at=now(), completed_at=now() WHERE id=$1`,
			id, string(status), errorMsg)
	default:
		_, err = s.pool.Exec(ctx,
			`UPDATE ai_jobs SET status=$2, error_message=NULLIF($3,''), updated_at=now() WHERE id=$1`,
			id, string(status), errorMsg)
	}
	if err != nil {
		return fmt.Errorf("UpdateAIJobStatus: %w", err)
	}
	return nil
}

func (s *Store) ListPendingAIJobs(ctx context.Context, limit int) ([]domain.AIJob, error) {
	const query = `
		SELECT id, team_id, author_user_id, job_type, status, payload, result, coalesce(error_message, ''), created_at, updated_at, completed_at
		FROM ai_jobs
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1
	`
	return s.listAIJobs(ctx, query, limit)
}

func (s *Store) listAIJobs(ctx context.Context, query string, args ...any) ([]domain.AIJob, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []domain.AIJob
	for rows.Next() {
		job, err := scanAIJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func scanAIJob(row interface{ Scan(dest ...any) error }) (domain.AIJob, error) {
	var job domain.AIJob
	var payloadRaw []byte
	var resultRaw []byte
	var completedAt sql.NullTime
	if err := row.Scan(
		&job.ID,
		&job.TeamID,
		&job.AuthorUserID,
		&job.Type,
		&job.Status,
		&payloadRaw,
		&resultRaw,
		&job.ErrorMessage,
		&job.CreatedAt,
		&job.UpdatedAt,
		&completedAt,
	); err != nil {
		return domain.AIJob{}, err
	}
	job.Payload = json.RawMessage(payloadRaw)
	if len(resultRaw) > 0 {
		job.Result = json.RawMessage(resultRaw)
	}
	if completedAt.Valid {
		t := completedAt.Time.UTC()
		job.CompletedAt = &t
	}
	return job, nil
}

func (s *Store) GetAIServiceConfig(ctx context.Context, teamID string) (domain.AIServiceConfig, error) {
	const query = `
		SELECT id, team_id, provider, model, base_url, api_key_ciphertext, description, created_at
		FROM ai_service_configs
		WHERE team_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	cfg, err := s.scanAIServiceConfig(s.pool.QueryRow(ctx, query, teamID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.AIServiceConfig{}, fmt.Errorf("ai service config not found: %w", pgx.ErrNoRows)
		}
		return domain.AIServiceConfig{}, fmt.Errorf("GetAIServiceConfig: %w", err)
	}
	return cfg, nil
}

func (s *Store) UpsertAIServiceConfig(ctx context.Context, teamID string, input domain.AIServiceConfig) (domain.AIServiceConfig, error) {
	apiKeyCiphertext := ""
	if strings.TrimSpace(input.APIKey) != "" {
		var encErr error
		apiKeyCiphertext, encErr = s.encrypter.Encrypt(input.APIKey)
		if encErr != nil {
			return domain.AIServiceConfig{}, fmt.Errorf("UpsertAIServiceConfig: encrypt api key: %w", encErr)
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.AIServiceConfig{}, fmt.Errorf("UpsertAIServiceConfig: %w", err)
	}
	defer tx.Rollback(ctx)

	// Empty API key on update keeps the stored key.
	if apiKeyCiphertext == "" {
		var existing string
		err := tx.QueryRow(ctx, `SELECT api_key_ciphertext FROM ai_service_configs WHERE team_id = $1 ORDER BY created_at DESC LIMIT 1`, teamID).Scan(&existing)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return domain.AIServiceConfig{}, fmt.Errorf("UpsertAIServiceConfig: read existing key: %w", err)
		}
		apiKeyCiphertext = existing
	}

	if _, err := tx.Exec(ctx, `DELETE FROM ai_service_configs WHERE team_id = $1`, teamID); err != nil {
		return domain.AIServiceConfig{}, fmt.Errorf("UpsertAIServiceConfig: delete: %w", err)
	}

	const insertQuery = `
		INSERT INTO ai_service_configs (team_id, provider, model, base_url, api_key_ciphertext, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, team_id, provider, model, base_url, api_key_ciphertext, description, created_at
	`
	cfg, err := s.scanAIServiceConfig(tx.QueryRow(ctx, insertQuery, teamID, input.Provider, input.Model, input.BaseURL, apiKeyCiphertext, input.Description))
	if err != nil {
		return domain.AIServiceConfig{}, fmt.Errorf("UpsertAIServiceConfig: insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.AIServiceConfig{}, fmt.Errorf("UpsertAIServiceConfig: commit: %w", err)
	}
	return cfg, nil
}

func (s *Store) scanAIServiceConfig(row interface{ Scan(dest ...any) error }) (domain.AIServiceConfig, error) {
	var cfg domain.AIServiceConfig
	var teamID sql.NullString
	var apiKeyCiphertext string
	if err := row.Scan(
		&cfg.ID,
		&teamID,
		&cfg.Provider,
		&cfg.Model,
		&cfg.BaseURL,
		&apiKeyCiphertext,
		&cfg.Description,
		&cfg.CreatedAt,
	); err != nil {
		return domain.AIServiceConfig{}, err
	}
	if teamID.Valid {
		cfg.TeamID = &teamID.String
	}
	if apiKeyCiphertext != "" {
		apiKey, err := s.encrypter.Decrypt(apiKeyCiphertext)
		if err != nil {
			return domain.AIServiceConfig{}, fmt.Errorf("decrypt ai api key: %w", err)
		}
		cfg.APIKey = apiKey
		cfg.APIKeySet = true
	}
	return cfg, nil
}

func (s *Store) CreateRSSFeedConfig(ctx context.Context, teamID string, input domain.RSSFeedConfig) (domain.RSSFeedConfig, error) {
	targetJSON, err := encodeMediaIDsJSON(domain.NormalizeMediaIDs(input.TargetAccountIDs))
	if err != nil {
		return domain.RSSFeedConfig{}, fmt.Errorf("CreateRSSFeedConfig: %w", err)
	}
	syncMode := string(domain.NormalizeRSSInitialSyncMode(string(input.InitialSyncMode)))
	contentTemplate := input.NormalizedContentTemplate()
	outputMode := string(domain.NormalizeAutomationOutputMode(string(input.OutputMode)))
	maxPosts := input.NormalizedMaxPostsPerDay()
	titleTemplate := input.NormalizedTitleTemplate()
	query := `
		INSERT INTO rss_feed_configs (
			team_id, feed_url, name, is_active, ai_enhance_enabled, content_template, title_template, title_hint, output_mode, max_posts_per_day,
			prompt_hint, target_account_ids, tonality, initial_sync_mode
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING ` + strings.TrimSpace(rssFeedConfigSelectList)
	feed, err := scanRSSFeedConfig(s.pool.QueryRow(ctx, query,
		teamID, input.FeedURL, input.Name, input.IsActive, input.AiEnhanceEnabled, contentTemplate, titleTemplate, input.TitleHint, outputMode, maxPosts,
		input.PromptHint, targetJSON, input.Tonality, syncMode,
	))
	if err != nil {
		return domain.RSSFeedConfig{}, fmt.Errorf("CreateRSSFeedConfig: %w", err)
	}
	return feed, nil
}

func (s *Store) GetRSSFeedConfigByID(ctx context.Context, teamID string, id string) (domain.RSSFeedConfig, error) {
	query := `SELECT ` + strings.TrimSpace(rssFeedConfigSelectList) + `
		FROM rss_feed_configs
		WHERE team_id = $1 AND id = $2`
	feed, err := scanRSSFeedConfig(s.pool.QueryRow(ctx, query, teamID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RSSFeedConfig{}, fmt.Errorf("rss feed config not found: %w", pgx.ErrNoRows)
		}
		return domain.RSSFeedConfig{}, fmt.Errorf("GetRSSFeedConfigByID: %w", err)
	}
	return feed, nil
}

func (s *Store) ListRSSFeedConfigs(ctx context.Context, teamID string) ([]domain.RSSFeedConfig, error) {
	query := `SELECT ` + strings.TrimSpace(rssFeedConfigSelectList) + `
		FROM rss_feed_configs
		WHERE team_id = $1
		ORDER BY created_at ASC`
	rows, err := s.pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("ListRSSFeedConfigs: %w", err)
	}
	defer rows.Close()

	var feeds []domain.RSSFeedConfig
	for rows.Next() {
		feed, err := scanRSSFeedConfig(rows)
		if err != nil {
			return nil, fmt.Errorf("ListRSSFeedConfigs: scan: %w", err)
		}
		feeds = append(feeds, feed)
	}
	return feeds, rows.Err()
}

func (s *Store) UpdateRSSFeedConfig(ctx context.Context, teamID string, id string, input domain.RSSFeedConfig) (domain.RSSFeedConfig, error) {
	targetJSON, err := encodeMediaIDsJSON(domain.NormalizeMediaIDs(input.TargetAccountIDs))
	if err != nil {
		return domain.RSSFeedConfig{}, fmt.Errorf("UpdateRSSFeedConfig: %w", err)
	}
	contentTemplate := input.NormalizedContentTemplate()
	titleTemplate := input.NormalizedTitleTemplate()
	outputMode := string(domain.NormalizeAutomationOutputMode(string(input.OutputMode)))
	maxPosts := input.NormalizedMaxPostsPerDay()
	query := `
		UPDATE rss_feed_configs
		SET feed_url = $3, name = $4, is_active = $5, ai_enhance_enabled = $6, content_template = $7, title_template = $8, title_hint = $9,
		    output_mode = $10, max_posts_per_day = $11, prompt_hint = $12, target_account_ids = $13, tonality = $14,
		    initial_sync_mode = $15, last_fetched_at = COALESCE($16, last_fetched_at)
		WHERE team_id = $1 AND id = $2
		RETURNING ` + strings.TrimSpace(rssFeedConfigSelectList)
	syncMode := string(domain.NormalizeRSSInitialSyncMode(string(input.InitialSyncMode)))
	feed, err := scanRSSFeedConfig(s.pool.QueryRow(ctx, query,
		teamID, id, input.FeedURL, input.Name, input.IsActive, input.AiEnhanceEnabled, contentTemplate, titleTemplate, input.TitleHint, outputMode, maxPosts,
		input.PromptHint, targetJSON, input.Tonality, syncMode, input.LastFetchedAt,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RSSFeedConfig{}, fmt.Errorf("rss feed config not found: %w", pgx.ErrNoRows)
		}
		return domain.RSSFeedConfig{}, fmt.Errorf("UpdateRSSFeedConfig: %w", err)
	}
	return feed, nil
}

func (s *Store) DeleteRSSFeedConfig(ctx context.Context, teamID string, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM rss_feed_configs WHERE team_id = $1 AND id = $2`, teamID, id)
	if err != nil {
		return fmt.Errorf("DeleteRSSFeedConfig: %w", err)
	}
	return nil
}

func scanRSSFeedConfig(row interface{ Scan(dest ...any) error }) (domain.RSSFeedConfig, error) {
	var feed domain.RSSFeedConfig
	var targetRaw string
	var lastFetched sql.NullTime
	var syncMode string
	var outputMode string
	if err := row.Scan(
		&feed.ID,
		&feed.TeamID,
		&feed.FeedURL,
		&feed.Name,
		&feed.IsActive,
		&feed.AiEnhanceEnabled,
		&feed.ContentTemplate,
		&feed.TitleTemplate,
		&feed.TitleHint,
		&outputMode,
		&feed.MaxPostsPerDay,
		&feed.CounterNext,
		&feed.PromptHint,
		&targetRaw,
		&feed.Tonality,
		&syncMode,
		&lastFetched,
		&feed.CreatedAt,
	); err != nil {
		return domain.RSSFeedConfig{}, err
	}
	if err := decodePostMediaIDs(targetRaw, &feed.TargetAccountIDs); err != nil {
		return domain.RSSFeedConfig{}, err
	}
	feed.OutputMode = domain.NormalizeAutomationOutputMode(outputMode)
	feed.InitialSyncMode = domain.NormalizeRSSInitialSyncMode(syncMode)
	if lastFetched.Valid {
		t := lastFetched.Time.UTC()
		feed.LastFetchedAt = &t
	}
	return feed, nil
}

func (s *Store) GetProactiveTriggerSettings(ctx context.Context, teamID string) (domain.ProactiveTriggerSettings, error) {
	const query = `
		SELECT id, team_id, content_gap_threshold_days, auto_fill_enabled, max_triggers_per_day, cron_schedule, created_at, updated_at
		FROM proactive_trigger_settings
		WHERE team_id = $1
	`
	pts, err := scanProactiveTriggerSettings(s.pool.QueryRow(ctx, query, teamID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ProactiveTriggerSettings{}, fmt.Errorf("proactive trigger settings not found: %w", pgx.ErrNoRows)
		}
		return domain.ProactiveTriggerSettings{}, fmt.Errorf("GetProactiveTriggerSettings: %w", err)
	}
	return pts, nil
}

func (s *Store) UpsertProactiveTriggerSettings(ctx context.Context, teamID string, input domain.ProactiveTriggerSettings) (domain.ProactiveTriggerSettings, error) {
	const query = `
		INSERT INTO proactive_trigger_settings (team_id, content_gap_threshold_days, auto_fill_enabled, max_triggers_per_day, cron_schedule)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (team_id) DO UPDATE
		SET content_gap_threshold_days = EXCLUDED.content_gap_threshold_days,
		    auto_fill_enabled = EXCLUDED.auto_fill_enabled,
		    max_triggers_per_day = EXCLUDED.max_triggers_per_day,
		    cron_schedule = EXCLUDED.cron_schedule,
		    updated_at = now()
		RETURNING id, team_id, content_gap_threshold_days, auto_fill_enabled, max_triggers_per_day, cron_schedule, created_at, updated_at
	`
	pts, err := scanProactiveTriggerSettings(s.pool.QueryRow(ctx, query,
		teamID,
		input.ContentGapThresholdDays,
		input.AutoFillEnabled,
		input.MaxTriggersPerDay,
		input.CronSchedule,
	))
	if err != nil {
		return domain.ProactiveTriggerSettings{}, fmt.Errorf("UpsertProactiveTriggerSettings: %w", err)
	}
	return pts, nil
}

func scanProactiveTriggerSettings(row interface{ Scan(dest ...any) error }) (domain.ProactiveTriggerSettings, error) {
	var pts domain.ProactiveTriggerSettings
	if err := row.Scan(
		&pts.ID,
		&pts.TeamID,
		&pts.ContentGapThresholdDays,
		&pts.AutoFillEnabled,
		&pts.MaxTriggersPerDay,
		&pts.CronSchedule,
		&pts.CreatedAt,
		&pts.UpdatedAt,
	); err != nil {
		return domain.ProactiveTriggerSettings{}, err
	}
	return pts, nil
}

func (s *Store) GetTeamAIContext(ctx context.Context, teamID string) (domain.AIContext, error) {
	team, err := s.GetTeamByID(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, fmt.Errorf("GetTeamAIContext: get team: %w", err)
	}

	var profilePtr *domain.TeamProfile
	profile, err := s.GetTeamProfile(ctx, teamID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return domain.AIContext{}, fmt.Errorf("GetTeamAIContext: get profile: %w", err)
		}
	} else {
		profilePtr = &profile
	}

	formats, err := s.ListCampaignFormats(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, fmt.Errorf("GetTeamAIContext: list formats: %w", err)
	}

	examples, err := s.ListStyleExamples(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, fmt.Errorf("GetTeamAIContext: list style examples: %w", err)
	}

	knowledgeSources, err := s.ListKnowledgeSources(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, fmt.Errorf("GetTeamAIContext: list knowledge sources: %w", err)
	}

	const recentPostsQuery = `
		SELECT p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status, p.source,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       p.visibility, p.media_ids, coalesce(p.media_exclude_by_account::text, '{}'),
		       p.post_template_id::text, p.template_counter,
		       coalesce(array_agg(t.account_id::text) filter (where t.account_id is not null), '{}')
		FROM scheduled_posts p
		LEFT JOIN scheduled_post_targets t ON t.post_id = p.id
		WHERE p.team_id = $1 AND p.status = 'posted' AND trim(p.content) <> ''
		GROUP BY p.id
		ORDER BY p.scheduled_at DESC
		LIMIT $2
	`
	recentPosts, err := s.listPosts(ctx, recentPostsQuery, teamID, domain.AIContextRecentPostsLimit)
	if err != nil {
		return domain.AIContext{}, fmt.Errorf("GetTeamAIContext: list recent posts: %w", err)
	}
	if recentPosts == nil {
		recentPosts = []domain.ScheduledPost{}
	}

	const upcomingPostsQuery = `
		SELECT p.id, p.team_id, p.author_user_id, p.title, p.content, p.scheduled_at, p.status, p.source,
		       p.attempt_count, coalesce(p.last_error, ''), p.created_at, p.updated_at,
		       p.visibility, p.media_ids, coalesce(p.media_exclude_by_account::text, '{}'),
		       p.post_template_id::text, p.template_counter,
		       coalesce(array_agg(t.account_id::text) filter (where t.account_id is not null), '{}')
		FROM scheduled_posts p
		LEFT JOIN scheduled_post_targets t ON t.post_id = p.id
		WHERE p.team_id = $1 AND p.status IN ('pending', 'draft') AND p.scheduled_at >= now()
		GROUP BY p.id
		ORDER BY p.scheduled_at ASC
		LIMIT $2
	`
	upcomingPosts, err := s.listPosts(ctx, upcomingPostsQuery, teamID, domain.AIContextUpcomingPostsLimit)
	if err != nil {
		return domain.AIContext{}, fmt.Errorf("GetTeamAIContext: list upcoming posts: %w", err)
	}
	if upcomingPosts == nil {
		upcomingPosts = []domain.ScheduledPost{}
	}

	accounts, err := s.ListTeamAccounts(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, fmt.Errorf("GetTeamAIContext: list accounts: %w", err)
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
		return domain.AIContext{}, fmt.Errorf("GetTeamAIContext: engagement hours: %w", err)
	}

	topHashtags, err := s.ListTeamHashtagPerformance(ctx, teamID, 90, "", 20)
	if err != nil {
		return domain.AIContext{}, fmt.Errorf("GetTeamAIContext: top hashtags: %w", err)
	}

	if formats == nil {
		formats = []domain.CampaignFormat{}
	}
	if examples == nil {
		examples = []domain.StyleExample{}
	}
	if knowledgeSources == nil {
		knowledgeSources = []domain.KnowledgeSource{}
	}

	return domain.AIContext{
		Team:             team,
		Profile:          profilePtr,
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
	const query = `
		SELECT id, name, description, created_at, is_personal, is_ai_enabled, personal_for_user_id, scheduling_prefs, brand_color
		FROM teams
		WHERE is_ai_enabled = true
		ORDER BY name ASC
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ListAIEnabledTeams: %w", err)
	}
	defer rows.Close()

	var teams []domain.Team
	for rows.Next() {
		team, err := scanTeamRow(rows)
		if err != nil {
			return nil, fmt.Errorf("ListAIEnabledTeams: scan: %w", err)
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}
