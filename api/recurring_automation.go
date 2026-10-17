package api

import (
	"encoding/json"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

type recurringAutomationMeta struct {
	TemplateID             string
	PostKind               string
	OutputMode             domain.AutomationOutputMode
	ScheduledAt            time.Time
	TemplateOccurrenceAt   *time.Time
	Draft                  bool
	PostTitle              string
	FallbackContent        string
	TemplateCounter        int
	Visibility             string
	MediaIDs               []string
	MediaExcludeByAccount  map[string][]string
	HasPublicationSnapshot bool
}

func parseRecurringAutomationMeta(payload json.RawMessage) *recurringAutomationMeta {
	if len(payload) == 0 {
		return nil
	}
	var envelope struct {
		Params struct {
			RecurringAutomation *struct {
				TemplateID            string              `json:"template_id"`
				PostKind              string              `json:"post_kind"`
				OutputMode            string              `json:"output_mode"`
				ScheduledAt           string              `json:"scheduled_at"`
				TemplateOccurrenceAt  string              `json:"template_occurrence_at"`
				Draft                 bool                `json:"draft"`
				PostTitle             string              `json:"post_title"`
				FallbackContent       string              `json:"fallback_content"`
				TemplateCounter       int                 `json:"template_counter"`
				Visibility            *string             `json:"visibility"`
				MediaIDs              []string            `json:"media_ids"`
				MediaExcludeByAccount map[string][]string `json:"media_exclude_by_account"`
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
	postKind := strings.TrimSpace(raw.PostKind)
	if postKind == "" {
		postKind = "main"
	}
	var occurrenceAt *time.Time
	if raw.TemplateOccurrenceAt != "" {
		if parsed, err := time.Parse(time.RFC3339, raw.TemplateOccurrenceAt); err == nil {
			u := parsed.UTC()
			occurrenceAt = &u
		}
	}
	visibility := ""
	if raw.Visibility != nil {
		visibility = *raw.Visibility
	}
	return &recurringAutomationMeta{
		TemplateID:            raw.TemplateID,
		PostKind:              postKind,
		OutputMode:            domain.NormalizeAutomationOutputMode(raw.OutputMode),
		ScheduledAt:           scheduledAt,
		TemplateOccurrenceAt:  occurrenceAt,
		Draft:                 raw.Draft,
		PostTitle:             strings.TrimSpace(raw.PostTitle),
		FallbackContent:       strings.TrimSpace(raw.FallbackContent),
		TemplateCounter:       counter,
		Visibility:            visibility,
		MediaIDs:              raw.MediaIDs,
		MediaExcludeByAccount: raw.MediaExcludeByAccount,
		// Scheduler snapshots always include visibility, even when empty or public.
		HasPublicationSnapshot: raw.Visibility != nil,
	}
}

func recurringAutomationDraftFromMeta(meta *recurringAutomationMeta) bool {
	if meta == nil {
		return true
	}
	if meta.PostKind == "announcement" {
		return false
	}
	if meta.OutputMode == domain.AutomationOutputDraft {
		return true
	}
	return meta.Draft
}
