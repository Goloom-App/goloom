package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
)

func (s *Service) shouldEnhanceRecurringWithAI(ctx context.Context, tmpl domain.PostTemplate) bool {
	if !tmpl.AiEnhanceEnabled || s.jobManager == nil {
		return false
	}
	team, err := s.store.GetTeamByID(ctx, tmpl.TeamID)
	if err != nil || !team.IsAIEnabled {
		return false
	}
	return true
}

func (s *Service) submitRecurringAIEnhancement(
	ctx context.Context,
	tmpl domain.PostTemplate,
	expandedContent string,
	expandedTitle string,
	scheduledAt time.Time,
	draft bool,
) error {
	outputMode := domain.NormalizeAutomationOutputMode(string(tmpl.OutputMode))
	params := map[string]any{
		"refine_content":     true,
		"source_content":     expandedContent,
		"prompt_hint":        strings.TrimSpace(tmpl.PromptHint),
		"title_hint":         strings.TrimSpace(tmpl.TitleHint),
		"tonality":           strings.TrimSpace(tmpl.Tonality),
		"target_account_ids": tmpl.TargetAccountIDs,
		"schedule":           false,
		"recurring_automation": map[string]any{
			"template_id":      tmpl.ID,
			"output_mode":      string(outputMode),
			"scheduled_at":     scheduledAt.UTC().Format(time.RFC3339),
			"draft":            draft,
			"post_title":       expandedTitle,
			"fallback_content": expandedContent,
			"template_counter": tmpl.CounterNext,
		},
	}
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		Params json.RawMessage `json:"params"`
	}{Params: paramsRaw})
	if err != nil {
		return err
	}

	job, err := s.jobManager.SubmitJob(ctx, domain.AIJob{
		TeamID:       tmpl.TeamID,
		AuthorUserID: tmpl.AuthorUserID,
		Type:         domain.AIJobTypeVoiceEngine,
		Payload:      payload,
	})
	if err != nil {
		if errors.Is(err, aijobs.ErrAIServiceNotConfigured) {
			return err
		}
		return fmt.Errorf("submit recurring ai job: %w", err)
	}
	if job.Status == domain.AIJobStatusFailed {
		return fmt.Errorf("recurring ai job dispatch failed")
	}
	s.logger.InfoContext(ctx, "recurring materialize: ai enhancement queued", "template_id", tmpl.ID, "job_id", job.ID)
	return nil
}
