package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/ai"
	"git.f4mily.net/goloom/internal/domain"
)

// handleAIChat streams an AI assistant conversation over SSE. The model can
// call tools that create drafts, campaigns, and automations for the team.
func (a *API) handleAIChat(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")

	var input domain.AIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if len(input.Messages) == 0 {
		a.writeError(w, r, "messages_required", http.StatusBadRequest)
		return
	}

	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	config, err := a.store.GetAIServiceConfig(r.Context(), teamID)
	if err != nil || strings.TrimSpace(config.APIKey) == "" {
		a.writeError(w, r, "ai_service_not_configured", http.StatusUnprocessableEntity)
		return
	}

	aiContext, err := a.store.GetTeamAIContext(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	client, err := ai.NewClient(ai.SettingsFromConfig(config), nil)
	if err != nil {
		a.writeError(w, r, "ai_service_not_configured", http.StatusUnprocessableEntity)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	mentionContext := a.chatMentionContext(r.Context(), teamID, input.Messages)
	system := ai.BuildChatSystemPrompt(aiContext, mentionContext)
	history := ai.ChatMessagesFromDomain(input.Messages)
	tools := a.chatTools(teamID, principal, aiContext)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	emit := func(event ai.ChatEvent) {
		data, err := json.Marshal(event)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	if err := ai.RunChat(r.Context(), client, system, history, tools, emit); err != nil {
		a.log.ErrorContext(r.Context(), "ai chat failed", "team_id", teamID, "error", err)
		emit(ai.ChatEvent{Type: "error", Message: "The AI provider request failed. Check the team AI configuration."})
	}
	emit(ai.ChatEvent{Type: "done"})
}

// chatMentionContext loads details for @-mentioned entities into prompt sections.
func (a *API) chatMentionContext(ctx context.Context, teamID string, messages []domain.AIChatMessage) []string {
	seen := map[string]bool{}
	var sections []string
	for _, message := range messages {
		for _, mention := range message.Mentions {
			key := string(mention.Type) + ":" + mention.ID
			if mention.ID == "" || seen[key] {
				continue
			}
			seen[key] = true
			switch mention.Type {
			case domain.AIChatMentionTypeCampaign:
				format, err := a.store.GetCampaignFormatByID(ctx, teamID, mention.ID)
				if err != nil {
					continue
				}
				data, _ := json.Marshal(format)
				sections = append(sections, fmt.Sprintf("Referenced campaign format %q:\n%s", format.Name, data))
			case domain.AIChatMentionTypeRecurring:
				tmpl, err := a.store.GetPostTemplate(ctx, teamID, mention.ID)
				if err != nil {
					continue
				}
				data, _ := json.Marshal(tmpl)
				sections = append(sections, fmt.Sprintf("Referenced recurring automation %q:\n%s", tmpl.Title, data))
			case domain.AIChatMentionTypeRSS:
				feed, err := a.store.GetRSSFeedConfigByID(ctx, teamID, mention.ID)
				if err != nil {
					continue
				}
				data, _ := json.Marshal(feed)
				sections = append(sections, fmt.Sprintf("Referenced RSS automation %q:\n%s", feed.Name, data))
			}
		}
	}
	return sections
}

func (a *API) chatTools(teamID string, principal domain.AuthenticatedPrincipal, aiContext domain.AIContext) []ai.ChatTool {
	defaultAccountIDs := make([]string, 0, len(aiContext.Accounts))
	for _, account := range aiContext.Accounts {
		defaultAccountIDs = append(defaultAccountIDs, account.ID)
	}

	return []ai.ChatTool{
		{
			Tool: ai.Tool{
				Name: "create_draft",
				Description: "Create a post draft in Goloom. Use this when the user asks you to draft or write a post. " +
					"The draft appears in the team's planning view; nothing is published.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"title": {"type": "string", "description": "Short internal title for editors (max 120 chars)"},
						"content": {"type": "string", "description": "The full post text including hashtags"},
						"target_account_ids": {"type": "array", "items": {"type": "string"}, "description": "Account IDs to target; omit to target all connected accounts"},
						"scheduled_at": {"type": "string", "description": "Optional RFC3339 publication time suggestion"}
					},
					"required": ["content"]
				}`),
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, json.RawMessage, error) {
				var in struct {
					Title            string   `json:"title"`
					Content          string   `json:"content"`
					TargetAccountIDs []string `json:"target_account_ids"`
					ScheduledAt      string   `json:"scheduled_at"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", nil, fmt.Errorf("invalid arguments: %w", err)
				}
				if strings.TrimSpace(in.Content) == "" {
					return "", nil, fmt.Errorf("content is required")
				}
				targets := domain.NormalizeMediaIDs(in.TargetAccountIDs)
				if len(targets) == 0 {
					targets = defaultAccountIDs
				}
				if len(targets) == 0 {
					return "", nil, fmt.Errorf("the team has no connected accounts")
				}
				scheduledAt := time.Now().UTC()
				if in.ScheduledAt != "" {
					if parsed, err := time.Parse(time.RFC3339, in.ScheduledAt); err == nil {
						scheduledAt = parsed.UTC()
					}
				}
				post, err := a.store.CreateScheduledPost(ctx, teamID, principal, domain.CreatePostInput{
					Title:          strings.TrimSpace(in.Title),
					Content:        in.Content,
					TargetAccounts: targets,
					ScheduledAt:    scheduledAt,
					Draft:          true,
					AuthorUserID:   &principal.User.ID,
				})
				if err != nil {
					return "", nil, err
				}
				payload, _ := json.Marshal(post)
				return fmt.Sprintf("Draft created with id %s.", post.ID), payload, nil
			},
		},
		{
			Tool: ai.Tool{
				Name: "create_campaign",
				Description: "Create a campaign format (a reusable post series definition, e.g. a weekly themed post). " +
					"Use when the user asks to set up a campaign.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"name": {"type": "string", "description": "Campaign name"},
						"weekday": {"type": "integer", "minimum": 0, "maximum": 6, "description": "Preferred publication weekday (0=Sunday … 6=Saturday)"},
						"structure": {"type": "object", "description": "Optional structure template, e.g. {\"hook\": \"...\", \"body\": \"...\", \"cta\": \"...\"}"},
						"required_hashtags": {"type": "array", "items": {"type": "string"}, "description": "Hashtags every campaign post must contain"}
					},
					"required": ["name"]
				}`),
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, json.RawMessage, error) {
				var in struct {
					Name             string          `json:"name"`
					Weekday          *int            `json:"weekday"`
					Structure        json.RawMessage `json:"structure"`
					RequiredHashtags []string        `json:"required_hashtags"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", nil, fmt.Errorf("invalid arguments: %w", err)
				}
				if strings.TrimSpace(in.Name) == "" {
					return "", nil, fmt.Errorf("name is required")
				}
				structure := in.Structure
				if len(structure) == 0 {
					structure = json.RawMessage(`{}`)
				}
				format, err := a.store.CreateCampaignFormat(ctx, teamID, domain.CampaignFormat{
					Name:             strings.TrimSpace(in.Name),
					Weekday:          in.Weekday,
					Structure:        structure,
					RequiredHashtags: in.RequiredHashtags,
					IsActive:         true,
				})
				if err != nil {
					return "", nil, err
				}
				payload, _ := json.Marshal(format)
				return fmt.Sprintf("Campaign format %q created with id %s.", format.Name, format.ID), payload, nil
			},
		},
		{
			Tool: ai.Tool{
				Name: "create_recurring_automation",
				Description: "Create a recurring post automation that publishes on a weekly schedule. " +
					"Use when the user asks for a repeating post (e.g. every Friday at 18:00).",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"title": {"type": "string", "description": "Internal title of the automation"},
						"content": {"type": "string", "description": "Post template text; may contain {counter} and date placeholders"},
						"weekdays": {"type": "array", "items": {"type": "integer", "minimum": 0, "maximum": 6}, "description": "Weekdays to publish on (0=Sunday … 6=Saturday)"},
						"hour": {"type": "integer", "minimum": 0, "maximum": 23},
						"minute": {"type": "integer", "minimum": 0, "maximum": 59},
						"timezone": {"type": "string", "description": "IANA timezone, e.g. Europe/Berlin; defaults to UTC"},
						"target_account_ids": {"type": "array", "items": {"type": "string"}, "description": "Account IDs to target; omit for all accounts"},
						"ai_enhance": {"type": "boolean", "description": "Rewrite each occurrence with AI in the brand voice"},
						"prompt_hint": {"type": "string", "description": "Editorial direction for the AI enhancement"}
					},
					"required": ["title", "content", "weekdays", "hour"]
				}`),
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, json.RawMessage, error) {
				var in struct {
					Title            string   `json:"title"`
					Content          string   `json:"content"`
					Weekdays         []int    `json:"weekdays"`
					Hour             int      `json:"hour"`
					Minute           int      `json:"minute"`
					Timezone         string   `json:"timezone"`
					TargetAccountIDs []string `json:"target_account_ids"`
					AIEnhance        bool     `json:"ai_enhance"`
					PromptHint       string   `json:"prompt_hint"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", nil, fmt.Errorf("invalid arguments: %w", err)
				}
				if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Content) == "" || len(in.Weekdays) == 0 {
					return "", nil, fmt.Errorf("title, content, and weekdays are required")
				}
				timezone := strings.TrimSpace(in.Timezone)
				if timezone == "" {
					timezone = "UTC"
				}
				recurrence, err := json.Marshal(map[string]any{
					"kind":     "weekly",
					"weekdays": in.Weekdays,
					"hour":     in.Hour,
					"minute":   in.Minute,
					"timezone": timezone,
				})
				if err != nil {
					return "", nil, err
				}
				targets := domain.NormalizeMediaIDs(in.TargetAccountIDs)
				if len(targets) == 0 {
					targets = defaultAccountIDs
				}
				enabled := true
				tmpl, err := a.store.CreatePostTemplate(ctx, teamID, principal, domain.CreatePostTemplateInput{
					Title:            in.Title,
					Content:          in.Content,
					RecurrenceJSON:   string(recurrence),
					TargetAccountIDs: targets,
					Enabled:          &enabled,
					AiEnhanceEnabled: &in.AIEnhance,
					PromptHint:       in.PromptHint,
				})
				if err != nil {
					return "", nil, err
				}
				payload, _ := json.Marshal(tmpl)
				return fmt.Sprintf("Recurring automation %q created with id %s.", tmpl.Title, tmpl.ID), payload, nil
			},
		},
		{
			Tool: ai.Tool{
				Name: "create_rss_automation",
				Description: "Create an RSS feed automation that turns new feed items into posts. " +
					"Use when the user wants to post from an RSS/Atom feed URL.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"feed_url": {"type": "string", "description": "The RSS/Atom feed URL"},
						"name": {"type": "string", "description": "Display name of the automation"},
						"output_mode": {"type": "string", "enum": ["draft", "scheduled", "publish_now"], "description": "What happens with generated posts; default draft"},
						"ai_enhance": {"type": "boolean", "description": "Rewrite items with AI in the brand voice"},
						"prompt_hint": {"type": "string", "description": "Editorial direction for the AI enhancement"},
						"target_account_ids": {"type": "array", "items": {"type": "string"}, "description": "Account IDs to target; omit for all accounts"}
					},
					"required": ["feed_url", "name"]
				}`),
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, json.RawMessage, error) {
				var in struct {
					FeedURL          string   `json:"feed_url"`
					Name             string   `json:"name"`
					OutputMode       string   `json:"output_mode"`
					AIEnhance        bool     `json:"ai_enhance"`
					PromptHint       string   `json:"prompt_hint"`
					TargetAccountIDs []string `json:"target_account_ids"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", nil, fmt.Errorf("invalid arguments: %w", err)
				}
				if strings.TrimSpace(in.FeedURL) == "" || strings.TrimSpace(in.Name) == "" {
					return "", nil, fmt.Errorf("feed_url and name are required")
				}
				targets := domain.NormalizeMediaIDs(in.TargetAccountIDs)
				if len(targets) == 0 {
					targets = defaultAccountIDs
				}
				feed, err := a.store.CreateRSSFeedConfig(ctx, teamID, domain.RSSFeedConfig{
					FeedURL:          strings.TrimSpace(in.FeedURL),
					Name:             strings.TrimSpace(in.Name),
					IsActive:         true,
					AiEnhanceEnabled: in.AIEnhance,
					ContentTemplate:  domain.DefaultRSSContentTemplate,
					TitleTemplate:    domain.DefaultRSSTitleTemplate,
					OutputMode:       domain.NormalizeAutomationOutputMode(in.OutputMode),
					MaxPostsPerDay:   10,
					PromptHint:       in.PromptHint,
					TargetAccountIDs: targets,
					InitialSyncMode:  domain.RSSInitialSyncBaseline,
				})
				if err != nil {
					return "", nil, err
				}
				payload, _ := json.Marshal(feed)
				return fmt.Sprintf("RSS automation %q created with id %s.", feed.Name, feed.ID), payload, nil
			},
		},
	}
}
