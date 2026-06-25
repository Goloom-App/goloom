package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
)

const (
	chatMaxIterations = 8
	chatMaxTokens     = 2000
)

// ChatEvent is one server-sent event emitted while the chat loop runs.
type ChatEvent struct {
	Type     string          `json:"type"` // status | message | tool_call | tool_result | error | done
	Message  string          `json:"message,omitempty"`
	ToolName string          `json:"tool_name,omitempty"`
	ToolArgs json.RawMessage `json:"tool_args,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

// ChatTool couples a tool definition with its executor. The executor returns
// the textual result fed back to the model and an optional payload that is
// forwarded to the frontend (e.g. a created draft for the preview card).
type ChatTool struct {
	Tool
	Execute func(ctx context.Context, args json.RawMessage) (result string, payload json.RawMessage, err error)
}

// BuildChatSystemPrompt assembles the chat assistant system prompt from the
// team's brand voice and the entities referenced via mentions.
func BuildChatSystemPrompt(aiContext domain.AIContext, mentionContext []string, viewSummary string) string {
	var sb strings.Builder
	sb.WriteString("You are the Goloom AI assistant for the team ")
	sb.WriteString(fmt.Sprintf("%q", orDefault(aiContext.Team.Name, "unknown team")))
	sb.WriteString(". You are a social media agent specialised in this platform: you help plan, draft, and automate posts, and you can read the team's calendar, posts, analytics and brand profile to act on your own initiative in the user's interest.\n\n")
	sb.WriteString("Capabilities (via tools): see what the user is currently looking at (get_current_view), read the calendar, posts, analytics and best posting times, recall the team's brand profile, fetch web pages, create and update post drafts, create campaign formats, create recurring and RSS automations, and query hashtag performance.\n")
	if strings.TrimSpace(viewSummary) != "" {
		sb.WriteString("\nCurrent view: ")
		sb.WriteString(strings.TrimSpace(viewSummary))
		sb.WriteString("\n")
	}
	sb.WriteString("Rules:\n")
	sb.WriteString("- Stay on the platform and your work. Answer a brief factual aside if asked, but steer back to social media and this team; decline off-platform jobs with no social-media purpose (e.g. writing an essay or a manuscript).\n")
	sb.WriteString("- When the user refers to what they see ('this post', 'here', 'the one I'm editing'), call get_current_view to ground yourself before acting.\n")
	sb.WriteString("- Before you write post content, recall the team's voice with get_brand_profile and match its tone, wording and rules.\n")
	sb.WriteString("- Use a tool when the user asks you to create or change something; otherwise just answer.\n")
	sb.WriteString("- draft_post saves a draft and runs immediately. modify_post edits an existing draft or scheduled post — use it (with the post id) for any change to a post that already exists, never create a second draft.\n")
	sb.WriteString("- schedule_post, delete_post and the automation tools (create_recurring, create_rss_feed) are proposed for the user to confirm — they only run after the user clicks confirm. Propose them plainly; do not pretend they already happened.\n")
	sb.WriteString("- To tweak one platform only, call modify_post with just that account's account_content_override and leave the others untouched. The default text must fit every targeted account; per-account overrides are shorter variants for lower-limit platforms.\n")
	sb.WriteString("- Pass target_account_ids explicitly from the connected accounts listed below. Each account has its own character limit; either write one text within the smallest limit or pass shorter per-account versions via account_content_override.\n")
	sb.WriteString("- To put a post on a specific day (e.g. a campaign's weekday), pick the time yourself: use find_free_slot or get_calendar, then schedule_post with that time. Read get_campaign for the campaign's structure and required hashtags.\n")
	sb.WriteString("- Ask for missing required details instead of inventing them.\n")
	sb.WriteString("- When the user shares a URL, you MUST fetch it with fetch_url and base post content on the actual page text. Never guess or invent what a page contains; if the fetch fails, say so and ask for the key facts.\n")
	sb.WriteString("- When you draft or propose a post, do not repeat the full post text in your reply — the user sees a card.\n")
	sb.WriteString("- Reply in the user's language.\n\n")

	if len(aiContext.Accounts) > 0 {
		sb.WriteString("Connected accounts:\n")
		for _, account := range aiContext.Accounts {
			sb.WriteString(fmt.Sprintf("- %s (id=%s, %s, max %d chars)\n", orDefault(account.Username, account.ID), account.ID, account.Provider, account.MaxChars))
		}
		sb.WriteString("\n")
	}
	if len(aiContext.CampaignFormats) > 0 {
		sb.WriteString("Existing campaign formats:\n")
		for _, format := range aiContext.CampaignFormats {
			sb.WriteString(fmt.Sprintf("- %s (id=%s)\n", format.Name, format.ID))
		}
		sb.WriteString("\n")
	}
	if len(aiContext.TopHashtags) > 0 {
		sb.WriteString("Best-performing hashtags of this team (last 90 days, all platforms):\n")
		for _, tag := range aiContext.TopHashtags {
			sb.WriteString(fmt.Sprintf("- #%s (%d uses, avg %.1f engagement per post)\n", orDefault(tag.Display, tag.Tag), tag.Uses, tag.AvgEngagement))
		}
		sb.WriteString("When drafting posts, prefer hashtags from this list if they fit the topic — but only topically fitting ones; using none is better than forcing an unrelated tag. Use get_hashtag_performance to filter by platform or time window.\n\n")
	}
	for _, section := range mentionContext {
		if strings.TrimSpace(section) == "" {
			continue
		}
		sb.WriteString(section)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Brand voice for any post content you write:\n\n")
	sb.WriteString(buildBrandVoicePrompt(aiContext))
	return sb.String()
}

// ChatMessagesFromDomain converts API chat messages to LLM messages, folding
// mention metadata into the user text so the model sees what was referenced.
func ChatMessagesFromDomain(messages []domain.AIChatMessage) []Message {
	out := make([]Message, 0, len(messages))
	for _, msg := range messages {
		role := RoleUser
		if msg.Role == domain.AIChatMessageRoleAssistant {
			role = RoleAssistant
		}
		content := msg.Content
		if len(msg.Mentions) > 0 && role == RoleUser {
			var refs []string
			for _, mention := range msg.Mentions {
				refs = append(refs, fmt.Sprintf("%s %q (id=%s)", mention.Type, mention.Name, mention.ID))
			}
			content += "\n\n[Referenced entities: " + strings.Join(refs, ", ") + "]"
		}
		out = append(out, Message{Role: role, Content: content})
	}
	return out
}

// RunChat drives the tool-calling loop and emits events for each step.
func RunChat(ctx context.Context, client Client, system string, history []Message, tools []ChatTool, emit func(ChatEvent)) error {
	toolDefs := make([]Tool, 0, len(tools))
	toolsByName := map[string]ChatTool{}
	for _, tool := range tools {
		toolDefs = append(toolDefs, tool.Tool)
		toolsByName[tool.Name] = tool
	}

	messages := history
	for iteration := 0; iteration < chatMaxIterations; iteration++ {
		resp, err := client.Complete(ctx, Request{
			System:    system,
			Messages:  messages,
			Tools:     toolDefs,
			MaxTokens: chatMaxTokens,
		})
		if err != nil {
			return err
		}

		if resp.Content != "" {
			emit(ChatEvent{Type: "message", Message: resp.Content})
		}
		if len(resp.ToolCalls) == 0 {
			return nil
		}

		messages = append(messages, Message{Role: RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, call := range resp.ToolCalls {
			emit(ChatEvent{Type: "tool_call", ToolName: call.Name, ToolArgs: call.Arguments})

			tool, ok := toolsByName[call.Name]
			var result string
			var payload json.RawMessage
			var execErr error
			if !ok {
				execErr = fmt.Errorf("unknown tool %q", call.Name)
			} else {
				result, payload, execErr = tool.Execute(ctx, call.Arguments)
			}
			if execErr != nil {
				result = "Error: " + execErr.Error()
			}
			emit(ChatEvent{Type: "tool_result", ToolName: call.Name, Message: result, Payload: payload})

			messages = append(messages, Message{
				Role:       RoleTool,
				Content:    result,
				ToolCallID: call.ID,
				ToolName:   call.Name,
			})
		}
	}
	emit(ChatEvent{Type: "error", Message: "tool loop exceeded maximum iterations"})
	return nil
}
