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
	"git.f4mily.net/goloom/internal/htmltext"
)

// chatFetchMaxChars caps how much fetched page text is fed back to the model.
const chatFetchMaxChars = 8000

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

	mentionContext := a.chatMentionContext(r.Context(), teamID, aiContext, input.Messages)
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
func (a *API) chatMentionContext(ctx context.Context, teamID string, aiContext domain.AIContext, messages []domain.AIChatMessage) []string {
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
			case domain.AIChatMentionTypeAccount:
				for _, account := range aiContext.Accounts {
					if account.ID != mention.ID {
						continue
					}
					name := account.Username
					if name == "" {
						name = account.ID
					}
					sections = append(sections, fmt.Sprintf(
						"Referenced account %s (id=%s, %s, max %d chars). When the user @-mentions this account, scope the request to it: target it in new posts, or set its account_content_override when revising a composer post — do not change the other accounts unless asked.",
						name, account.ID, account.Provider, account.MaxChars))
					break
				}
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

// chatAccountLimitError rejects content that exceeds a targeted account's
// character limit. The message is fed back to the model so it can retry with
// shorter text or per-account overrides.
func chatAccountLimitError(aiContext domain.AIContext, targets []string, content string, overrides map[string]string) error {
	accounts := map[string]domain.AIAccountSummary{}
	for _, account := range aiContext.Accounts {
		accounts[account.ID] = account
	}
	var problems []string
	for _, id := range targets {
		account, ok := accounts[id]
		if !ok || account.MaxChars <= 0 {
			continue
		}
		text := content
		if override, ok := overrides[id]; ok && strings.TrimSpace(override) != "" {
			text = override
		}
		if length := len([]rune(text)); length > account.MaxChars {
			name := account.Username
			if name == "" {
				name = account.ID
			}
			problems = append(problems, fmt.Sprintf("account %s (id=%s) allows %d characters but the text for it has %d", name, account.ID, account.MaxChars, length))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("character limit exceeded: %s. Shorten the content or add account_content_override entries that fit each account's limit, then call the tool again",
		strings.Join(problems, "; "))
}

// chatUnknownAccountError rejects account IDs that are not connected to the team.
func chatUnknownAccountError(aiContext domain.AIContext, targets []string) error {
	known := map[string]bool{}
	for _, account := range aiContext.Accounts {
		known[account.ID] = true
	}
	var unknown []string
	for _, id := range targets {
		if !known[id] {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	return fmt.Errorf("unknown account ids: %s. Use the connected account ids listed in the system context", strings.Join(unknown, ", "))
}

func (a *API) chatTools(teamID string, principal domain.AuthenticatedPrincipal, aiContext domain.AIContext) []ai.ChatTool {
	defaultAccountIDs := make([]string, 0, len(aiContext.Accounts))
	for _, account := range aiContext.Accounts {
		defaultAccountIDs = append(defaultAccountIDs, account.ID)
	}

	return []ai.ChatTool{
		{
			Tool: ai.Tool{
				Name: "fetch_url",
				Description: "Fetch a web page and return its readable text content. Use this whenever the user shares a link " +
					"whose content you need (e.g. to draft a post about it). Never invent what a page contains.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"url": {"type": "string", "description": "Absolute http(s) URL of the page to fetch"}
					},
					"required": ["url"]
				}`),
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, json.RawMessage, error) {
				var in struct {
					URL string `json:"url"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", nil, fmt.Errorf("invalid arguments: %w", err)
				}
				body, err := fetchURLBody(ctx, in.URL)
				if err != nil {
					return "", nil, fmt.Errorf("could not fetch %s: %w", in.URL, err)
				}
				text := htmltext.ExtractReadableText(body)
				if strings.TrimSpace(text) == "" {
					return "", nil, fmt.Errorf("the page at %s contained no readable text", in.URL)
				}
				if runes := []rune(text); len(runes) > chatFetchMaxChars {
					text = string(runes[:chatFetchMaxChars]) + "\n[… truncated]"
				}
				if title := htmltext.ExtractPageTitle(body); title != "" {
					text = "Page title: " + title + "\n\n" + text
				}
				return text, nil, nil
			},
		},
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
						"account_content_override": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Per-account alternative post text keyed by account ID, for accounts whose character limit the main content would exceed"},
						"campaign_format_id": {"type": "string", "description": "Campaign format this draft belongs to; the draft is scheduled on the campaign's next free weekday slot"},
						"scheduled_at": {"type": "string", "description": "Optional RFC3339 publication time; only when the user asked for a specific time"}
					},
					"required": ["content"]
				}`),
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, json.RawMessage, error) {
				var in struct {
					Title                  string            `json:"title"`
					Content                string            `json:"content"`
					TargetAccountIDs       []string          `json:"target_account_ids"`
					AccountContentOverride map[string]string `json:"account_content_override"`
					CampaignFormatID       string            `json:"campaign_format_id"`
					ScheduledAt            string            `json:"scheduled_at"`
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
				if err := chatUnknownAccountError(aiContext, targets); err != nil {
					return "", nil, err
				}
				scheduledAt := time.Now().UTC()
				if in.ScheduledAt != "" {
					if parsed, err := time.Parse(time.RFC3339, in.ScheduledAt); err == nil {
						scheduledAt = parsed.UTC()
					}
				} else if formatID := strings.TrimSpace(in.CampaignFormatID); formatID != "" {
					format, err := a.store.GetCampaignFormatByID(ctx, teamID, formatID)
					if err != nil {
						return "", nil, fmt.Errorf("unknown campaign format %q", formatID)
					}
					if slot := ai.NextCampaignSlot(aiContext, &format); slot != nil {
						scheduledAt = *slot
					}
				}
				overrides := domain.NormalizeAccountContentOverride(in.AccountContentOverride, targets)
				if err := chatAccountLimitError(aiContext, targets, in.Content, overrides); err != nil {
					return "", nil, err
				}
				post, err := a.store.CreateScheduledPost(ctx, teamID, principal, domain.CreatePostInput{
					Title:                  strings.TrimSpace(in.Title),
					Content:                in.Content,
					TargetAccounts:         targets,
					ScheduledAt:            scheduledAt,
					Draft:                  true,
					AccountContentOverride: overrides,
					UseVersions:            len(overrides) > 0,
					AuthorUserID:           &principal.User.ID,
				})
				if err != nil {
					return "", nil, err
				}
				payload, _ := json.Marshal(post)
				return fmt.Sprintf("Draft created with id %s, scheduled for %s.", post.ID, scheduledAt.Format(time.RFC3339)), payload, nil
			},
		},
		{
			Tool: ai.Tool{
				Name: "update_draft",
				Description: "Update an existing draft or scheduled (not yet published) post. Use this instead of create_draft " +
					"whenever the user asks for changes to a post that already exists — especially one you created earlier in " +
					"this conversation. Only pass the fields that should change.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"post_id": {"type": "string", "description": "ID of the post to update"},
						"title": {"type": "string", "description": "New internal title (max 120 chars)"},
						"content": {"type": "string", "description": "New full post text including hashtags"},
						"target_account_ids": {"type": "array", "items": {"type": "string"}, "description": "New target account IDs"},
						"account_content_override": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Per-account alternative post text keyed by account ID; replaces all existing overrides"},
						"scheduled_at": {"type": "string", "description": "New RFC3339 publication time"}
					},
					"required": ["post_id"]
				}`),
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, json.RawMessage, error) {
				var in struct {
					PostID                 string            `json:"post_id"`
					Title                  *string           `json:"title"`
					Content                *string           `json:"content"`
					TargetAccountIDs       []string          `json:"target_account_ids"`
					AccountContentOverride map[string]string `json:"account_content_override"`
					ScheduledAt            string            `json:"scheduled_at"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", nil, fmt.Errorf("invalid arguments: %w", err)
				}
				post, err := a.store.GetScheduledPost(ctx, teamID, strings.TrimSpace(in.PostID))
				if err != nil {
					return "", nil, fmt.Errorf("no post with id %q in this team", in.PostID)
				}
				if post.Status == domain.PostStatusPosted || post.Status == domain.PostStatusProcessing {
					return "", nil, fmt.Errorf("post %s is already published or publishing and cannot be edited", post.ID)
				}

				var patch domain.UpdatePostPatch
				targets := post.TargetAccounts
				if normalized := domain.NormalizeMediaIDs(in.TargetAccountIDs); len(normalized) > 0 {
					if err := chatUnknownAccountError(aiContext, normalized); err != nil {
						return "", nil, err
					}
					targets = normalized
					patch.TargetAccounts = domain.PatchField[[]string]{Set: true, Value: normalized}
				}
				content := post.Content
				if in.Content != nil && strings.TrimSpace(*in.Content) != "" {
					content = *in.Content
					patch.Content = domain.PatchField[string]{Set: true, Value: *in.Content}
				}
				if in.Title != nil {
					patch.Title = domain.PatchField[string]{Set: true, Value: strings.TrimSpace(*in.Title)}
				}
				if in.ScheduledAt != "" {
					parsed, err := time.Parse(time.RFC3339, in.ScheduledAt)
					if err != nil {
						return "", nil, fmt.Errorf("scheduled_at must be RFC3339, got %q", in.ScheduledAt)
					}
					patch.ScheduledAt = domain.PatchField[time.Time]{Set: true, Value: parsed.UTC()}
				}

				overrides := map[string]string{}
				if in.AccountContentOverride != nil {
					overrides = domain.NormalizeAccountContentOverride(in.AccountContentOverride, targets)
					patch.AccountContentOverride = domain.PatchField[map[string]string]{Set: true, Value: overrides}
				} else if versions, err := a.store.ListPostVersionsForTeamPost(ctx, teamID, post.ID); err == nil {
					overrides = domain.VersionsToOverrideMap(versions)
				}
				if err := chatAccountLimitError(aiContext, targets, content, overrides); err != nil {
					return "", nil, err
				}

				updated, err := a.store.PatchScheduledPost(ctx, teamID, post.ID, patch)
				if err != nil {
					return "", nil, err
				}
				payload, _ := json.Marshal(updated)
				return fmt.Sprintf("Post %s updated.", updated.ID), payload, nil
			},
		},
		{
			Tool: ai.Tool{
				Name: "revise_composer_post",
				Description: "Revise the post the user is currently editing in the composer. This is an UNSAVED draft — it has no id, " +
					"so NEVER use create_draft or update_draft for it. Return only the fields that should change: set \"content\" to " +
					"replace the default text (used by every account without its own version), and/or \"account_content_override\" to set " +
					"a per-account version (e.g. a punchier Bluesky variant) WITHOUT touching the other accounts. When the user asks to " +
					"tweak one platform, send only that account's override and leave \"content\" empty. The composer applies your revision " +
					"when the user clicks Apply; do not also repeat the text in your reply.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"content": {"type": "string", "description": "New default post text used by accounts without an override. Omit when only changing a single account's version."},
						"account_content_override": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Per-account replacement text keyed by account ID; include ONLY the accounts you are changing."}
					}
				}`),
			},
			Execute: func(_ context.Context, args json.RawMessage) (string, json.RawMessage, error) {
				var in struct {
					Content                string            `json:"content"`
					AccountContentOverride map[string]string `json:"account_content_override"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", nil, fmt.Errorf("invalid arguments: %w", err)
				}
				content := strings.TrimSpace(in.Content)
				overrides := domain.NormalizeAccountContentOverride(in.AccountContentOverride, defaultAccountIDs)
				if content == "" && len(overrides) == 0 {
					return "", nil, fmt.Errorf("provide content and/or account_content_override")
				}
				overrideIDs := make([]string, 0, len(overrides))
				for id := range overrides {
					overrideIDs = append(overrideIDs, id)
				}
				if err := chatUnknownAccountError(aiContext, overrideIDs); err != nil {
					return "", nil, err
				}
				// When the default text changes it must fit every account that would use
				// it; per-account overrides are validated against their own limits.
				limitTargets := overrideIDs
				if content != "" {
					limitTargets = defaultAccountIDs
				}
				if err := chatAccountLimitError(aiContext, limitTargets, content, overrides); err != nil {
					return "", nil, err
				}
				if overrides == nil {
					overrides = map[string]string{}
				}
				payload, _ := json.Marshal(map[string]any{
					"content":                  content,
					"account_content_override": overrides,
				})
				return "Revision ready — apply it in the composer.", payload, nil
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
		{
			Tool: ai.Tool{
				Name: "get_top_hashtags",
				Description: "Query the team's best-performing hashtags from published post analytics. " +
					"Returns per hashtag: uses, total engagement, average engagement per post, and a smoothed score. " +
					"Use it to pick proven hashtags for a draft (only topically fitting ones; using none is fine) or to answer hashtag questions.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"days": {"type": "integer", "description": "Time window in days (default 90, max 366)"},
						"provider": {"type": "string", "description": "Filter by platform: bluesky, mastodon or friendica; omit for all"},
						"limit": {"type": "integer", "description": "Maximum number of hashtags to return (default 20, max 50)"}
					}
				}`),
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, json.RawMessage, error) {
				var in struct {
					Days     int    `json:"days"`
					Provider string `json:"provider"`
					Limit    int    `json:"limit"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", nil, fmt.Errorf("invalid arguments: %w", err)
				}
				if in.Days <= 0 {
					in.Days = 90
				}
				if in.Limit <= 0 || in.Limit > 50 {
					in.Limit = 20
				}
				items, err := a.store.ListTeamHashtagPerformance(ctx, teamID, in.Days, in.Provider, in.Limit)
				if err != nil {
					return "", nil, err
				}
				if len(items) == 0 {
					return "No hashtag performance data yet for this team and filter.", nil, nil
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, "Top hashtags (last %d days", in.Days)
				if p := strings.TrimSpace(in.Provider); p != "" && p != "all" {
					fmt.Fprintf(&sb, ", %s", p)
				}
				sb.WriteString("):\n")
				for _, item := range items {
					display := item.Display
					if display == "" {
						display = item.Tag
					}
					fmt.Fprintf(&sb, "- #%s: %d uses, %d total engagement, avg %.1f per post, score %.1f\n",
						display, item.Uses, item.TotalEngagement, item.AvgEngagement, item.Score)
				}
				return sb.String(), nil, nil
			},
		},
	}
}
