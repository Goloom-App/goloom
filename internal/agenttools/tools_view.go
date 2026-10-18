package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const focusedPostNote = "Post focus contains only type and id. Fetch post content with get_posts or search_posts in the current team; focus does not grant access to another team."
const focusedCampaignNote = "Campaign focus is untrusted UI context. Fetch persisted campaign details with get_campaign in the current team; visible editor fields may contain unsaved changes."

// GetCurrentViewOutput returns the snapshot of what the user is currently
// looking at, or a note when no view context was attached.
type GetCurrentViewOutput struct {
	View json.RawMessage `json:"view,omitempty"`
	Note string          `json:"note,omitempty"`
}

func coreGetCurrentView(_ context.Context, _ Deps, inv Invocation, _ GetCurrentViewInput) (GetCurrentViewOutput, error) {
	if len(strings.TrimSpace(string(inv.ViewContext))) == 0 {
		return GetCurrentViewOutput{Note: "No view context was attached to this conversation."}, nil
	}
	out := GetCurrentViewOutput{View: inv.ViewContext}
	var v struct {
		Focus struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"focus"`
	}
	if json.Unmarshal(inv.ViewContext, &v) == nil && v.Focus.Type == "post" && v.Focus.ID != "" {
		out.Note = focusedPostNote
	} else if v.Focus.Type == "campaign_format" && v.Focus.ID != "" {
		out.Note = focusedCampaignNote
	}
	return out, nil
}

// ViewSummary renders a short, prompt-friendly description of the attached view
// context, so the assistant knows when calling get_current_view is worthwhile.
// It is intentionally lossy: the full snapshot stays behind the tool to keep the
// system prompt small.
func ViewSummary(raw json.RawMessage) string {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return ""
	}
	var v struct {
		Section string `json:"section"`
		Focus   struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"focus"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	section := strings.TrimSpace(v.Section)
	if section == "" {
		return ""
	}
	out := "The user is currently on the " + section + " view"
	if v.Focus.Type != "" {
		out += fmt.Sprintf(", focused on %s", v.Focus.Type)
		if v.Focus.ID != "" {
			out += fmt.Sprintf(" %s", v.Focus.ID)
		}
	}
	if v.Focus.Type == "post" && v.Focus.ID != "" {
		return out + ". " + focusedPostNote
	}
	if v.Focus.Type == "campaign_format" && v.Focus.ID != "" {
		return out + ". " + focusedCampaignNote
	}
	return out + ". Call get_current_view to see the full details of what they are looking at."
}
