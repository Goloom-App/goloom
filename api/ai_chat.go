package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/agenttools"
	"git.f4mily.net/goloom/internal/ai"
	"git.f4mily.net/goloom/internal/auth"
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
	system := ai.BuildChatSystemPrompt(aiContext, mentionContext, agenttools.ViewSummary(input.ViewContext))
	history := ai.ChatMessagesFromDomain(input.Messages)
	tools := a.chatTools()
	// The shared agent catalog adds the read/insight tools (calendar, posts,
	// search, analytics, brand recall, current view, …) the chat assistant
	// lacked, keeping it in sync with the MCP surface.
	tools = append(tools, agenttools.ChatTools(a.agentDeps(), agenttools.ChatBinding{
		TeamID:      teamID,
		Principal:   principal,
		ViewContext: input.ViewContext,
	})...)

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

// handleAIConfirmAction runs a write tool the assistant proposed but did not
// execute (scheduling, deletion, automations). The chat surfaces these as a
// confirmation card; this endpoint runs the action only after the user clicks
// confirm. Team access and scope are re-checked inside the tool, so the click is
// never a trust anchor.
func (a *API) handleAIConfirmAction(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")

	var body struct {
		Tool string          `json:"tool"`
		Args json.RawMessage `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Tool) == "" {
		a.writeError(w, r, "tool_required", http.StatusBadRequest)
		return
	}

	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	result, err := agenttools.RunConfirmed(r.Context(), a.agentDeps(), agenttools.ChatBinding{
		TeamID:    teamID,
		Principal: principal,
	}, body.Tool, body.Args)
	if err != nil {
		a.log.InfoContext(r.Context(), "ai confirm-action failed", "team_id", teamID, "tool", body.Tool, "error", err)
		a.writeError(w, r, "confirm_action_failed", http.StatusUnprocessableEntity)
		return
	}

	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"summary": result.Summary,
		"payload": result.Payload,
	})
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

// chatTools are the chat-only tools that are not part of the shared agent
// catalog. The shared read and write tools are appended by the caller via
// agenttools.ChatTools.
func (a *API) chatTools() []ai.ChatTool {
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
	}
}
