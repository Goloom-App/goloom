package api

import (
	"encoding/json"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

type recurringAutomationMeta struct {
	TemplateID      string
	OutputMode      domain.AutomationOutputMode
	ScheduledAt     time.Time
	Draft           bool
	PostTitle       string
	FallbackContent string
	TemplateCounter int
}

func parseRecurringAutomationMeta(payload json.RawMessage) *recurringAutomationMeta {
	if len(payload) == 0 {
		return nil
	}
	var envelope struct {
		Params struct {
			RecurringAutomation *struct {
				TemplateID      string `json:"template_id"`
				OutputMode      string `json:"output_mode"`
				ScheduledAt     string `json:"scheduled_at"`
				Draft           bool   `json:"draft"`
				PostTitle       string `json:"post_title"`
				FallbackContent string `json:"fallback_content"`
				TemplateCounter int    `json:"template_counter"`
			} `json:"recurring_automation"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Params.RecurringAutomation == nil {
		return nil
	}
	raw := envelope.Params.RecurringAutomation
	if strings.TrimSpace(raw.TemplateID) == "" {
		return nil
	}
	scheduledAt := time.Now().UTC()
	if raw.ScheduledAt != "" {
		if parsed, err := time.Parse(time.RFC3339, raw.ScheduledAt); err == nil {
			scheduledAt = parsed.UTC()
		}
	}
	counter := raw.TemplateCounter
	if counter <= 0 {
		counter = 1
	}
	return &recurringAutomationMeta{
		TemplateID:      raw.TemplateID,
		OutputMode:      domain.NormalizeAutomationOutputMode(raw.OutputMode),
		ScheduledAt:     scheduledAt,
		Draft:           raw.Draft,
		PostTitle:       strings.TrimSpace(raw.PostTitle),
		FallbackContent: strings.TrimSpace(raw.FallbackContent),
		TemplateCounter: counter,
	}
}

func recurringAutomationDraftFromMeta(meta *recurringAutomationMeta) bool {
	if meta == nil {
		return true
	}
	if meta.OutputMode == domain.AutomationOutputDraft {
		return true
	}
	return meta.Draft
}
