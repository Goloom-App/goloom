package api

import (
	"encoding/json"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

type rssAutomationMeta struct {
	FeedID          string
	ItemKey         string
	OutputMode      domain.AutomationOutputMode
	ScheduledAt     time.Time
	Draft           bool
	PostTitle       string
	FallbackContent string
}

func parseRSSAutomationMeta(payload json.RawMessage) *rssAutomationMeta {
	if len(payload) == 0 {
		return nil
	}
	var envelope struct {
		Params struct {
			RSSAutomation *struct {
				FeedID          string `json:"feed_id"`
				ItemKey         string `json:"item_key"`
				OutputMode      string `json:"output_mode"`
				ScheduledAt     string `json:"scheduled_at"`
				Draft           bool   `json:"draft"`
				PostTitle       string `json:"post_title"`
				FallbackContent string `json:"fallback_content"`
			} `json:"rss_automation"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Params.RSSAutomation == nil {
		return nil
	}
	raw := envelope.Params.RSSAutomation
	if strings.TrimSpace(raw.FeedID) == "" || strings.TrimSpace(raw.ItemKey) == "" {
		return nil
	}
	scheduledAt := time.Now().UTC()
	if raw.ScheduledAt != "" {
		if parsed, err := time.Parse(time.RFC3339, raw.ScheduledAt); err == nil {
			scheduledAt = parsed.UTC()
		}
	}
	return &rssAutomationMeta{
		FeedID:          raw.FeedID,
		ItemKey:         raw.ItemKey,
		OutputMode:      domain.NormalizeAutomationOutputMode(raw.OutputMode),
		ScheduledAt:     scheduledAt,
		Draft:           raw.Draft,
		PostTitle:       strings.TrimSpace(raw.PostTitle),
		FallbackContent: strings.TrimSpace(raw.FallbackContent),
	}
}

func rssAutomationDraftFromMeta(meta *rssAutomationMeta) bool {
	if meta == nil {
		return true
	}
	if meta.OutputMode == domain.AutomationOutputDraft {
		return true
	}
	return meta.Draft
}

func rssAutomationScheduledAt(meta *rssAutomationMeta, res aiCallbackResult) time.Time {
	if res.ScheduledAt != nil && !res.ScheduledAt.IsZero() {
		return res.ScheduledAt.UTC()
	}
	if res.SuggestedScheduledAt != nil && !res.SuggestedScheduledAt.IsZero() {
		return res.SuggestedScheduledAt.UTC()
	}
	if meta != nil && !meta.ScheduledAt.IsZero() {
		return meta.ScheduledAt.UTC()
	}
	return time.Now().UTC()
}
