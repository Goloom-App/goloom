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

const (
	recurringPostKindMain         = "main"
	recurringPostKindAnnouncement = "announcement"
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

func (s *Service) shouldEnhanceRecurringAnnouncementWithAI(ctx context.Context, tmpl domain.PostTemplate) bool {
	if !tmpl.AnnouncementEnabled || !tmpl.AiEnhanceEnabled || !tmpl.AiEnhanceAnnouncement || s.jobManager == nil {
		return false
	}
	if strings.TrimSpace(tmpl.AnnouncementContent) == "" {
		return false
	}
	team, err := s.store.GetTeamByID(ctx, tmpl.TeamID)
	if err != nil || !team.IsAIEnabled {
		return false
	}
	return true
}

func expandedAnnouncementReference(tmpl domain.PostTemplate, mainEventAt time.Time) (content, title string) {
	if !tmpl.AnnouncementEnabled || strings.TrimSpace(tmpl.AnnouncementContent) == "" {
		return "", ""
	}
	daysBefore := tmpl.AnnouncementDaysBefore
	if daysBefore <= 0 {
		daysBefore = 2
	}
	announceAt := mainEventAt.Add(-time.Duration(daysBefore) * 24 * time.Hour)
	counterVal := tmpl.AnnouncementCounterNext
	if counterVal < 1 {
		counterVal = 1
	}
	content = domain.ExpandDynamicVariables(tmpl.AnnouncementContent, announceAt, &counterVal, &mainEventAt)
	title = domain.ExpandPostTemplateTitle(tmpl.AnnouncementTitle, announceAt, counterVal, &mainEventAt)
	return content, title
}

func (s *Service) submitRecurringAIEnhancement(
	ctx context.Context,
	tmpl domain.PostTemplate,
	expandedContent string,
	expandedTitle string,
	scheduledAt time.Time,
	draft bool,
	mainEventAt *time.Time,
) error {
	annRefContent, annRefTitle := "", ""
	if mainEventAt != nil {
		annRefContent, annRefTitle = expandedAnnouncementReference(tmpl, *mainEventAt)
	}
	return s.submitRecurringAIJob(ctx, recurringAIJobInput{
		tmpl:            tmpl,
		postKind:        recurringPostKindMain,
		expandedContent: expandedContent,
		expandedTitle:   expandedTitle,
		scheduledAt:     scheduledAt,
		draft:           draft,
		targetAccounts:  tmpl.TargetAccountIDs,
		templateCounter: tmpl.CounterNext,
		annRefContent:   annRefContent,
		annRefTitle:     annRefTitle,
		mainEventAt:     mainEventAt,
	})
}

func (s *Service) submitRecurringAnnouncementAIEnhancement(
	ctx context.Context,
	tmpl domain.PostTemplate,
	expandedContent string,
	expandedTitle string,
	scheduledAt time.Time,
	mainEventAt time.Time,
) error {
	targets := tmpl.AnnouncementTargetAccountIDs
	if len(targets) == 0 {
		targets = tmpl.TargetAccountIDs
	}
	return s.submitRecurringAIJob(ctx, recurringAIJobInput{
		tmpl:            tmpl,
		postKind:        recurringPostKindAnnouncement,
		expandedContent: expandedContent,
		expandedTitle:   expandedTitle,
		scheduledAt:     scheduledAt,
		draft:           false,
		targetAccounts:  targets,
		templateCounter: tmpl.AnnouncementCounterNext,
		mainEventAt:     &mainEventAt,
	})
}

type recurringAIJobInput struct {
	tmpl            domain.PostTemplate
	postKind        string
	expandedContent string
	expandedTitle   string
	scheduledAt     time.Time
	draft           bool
	targetAccounts  []string
	templateCounter int
	annRefContent   string
	annRefTitle     string
	mainEventAt     *time.Time
}

func (s *Service) submitRecurringAIJob(ctx context.Context, in recurringAIJobInput) error {
	outputMode := domain.NormalizeAutomationOutputMode(string(in.tmpl.OutputMode))
	params := map[string]any{
		"refine_content":     true,
		"source_content":     in.expandedContent,
		"prompt_hint":        strings.TrimSpace(in.tmpl.PromptHint),
		"title_hint":         strings.TrimSpace(in.tmpl.TitleHint),
		"tonality":           strings.TrimSpace(in.tmpl.Tonality),
		"target_account_ids": in.targetAccounts,
		"schedule":           false,
		"recurring_post_kind": in.postKind,
	}
	if in.annRefContent != "" {
		params["announcement_reference_content"] = in.annRefContent
		if in.annRefTitle != "" {
			params["announcement_reference_title"] = in.annRefTitle
		}
	}
	if in.mainEventAt != nil {
		params["main_event_at"] = in.mainEventAt.UTC().Format(time.RFC3339)
	}
	params["recurring_automation"] = map[string]any{
		"template_id":      in.tmpl.ID,
		"post_kind":        in.postKind,
		"output_mode":      string(outputMode),
		"scheduled_at":     in.scheduledAt.UTC().Format(time.RFC3339),
		"draft":            in.draft,
		"post_title":       in.expandedTitle,
		"fallback_content": in.expandedContent,
		"template_counter": in.templateCounter,
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
		TeamID:       in.tmpl.TeamID,
		AuthorUserID: in.tmpl.AuthorUserID,
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
	s.logger.InfoContext(ctx, "recurring materialize: ai enhancement queued",
		"template_id", in.tmpl.ID, "job_id", job.ID, "post_kind", in.postKind)
	return nil
}
