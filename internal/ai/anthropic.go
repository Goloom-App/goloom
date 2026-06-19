package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type anthropicClient struct {
	settings Settings
	http     *http.Client
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

func (c *anthropicClient) Model() string { return c.settings.ResolvedModel() }

func (c *anthropicClient) Complete(ctx context.Context, req Request) (Response, error) {
	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch m.Role {
		case RoleTool:
			// Tool results are user-role messages with a tool_result block.
			messages = append(messages, anthropicMessage{
				Role: "user",
				Content: []anthropicContentBlock{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
		case RoleAssistant:
			blocks := []anthropicContentBlock{}
			if strings.TrimSpace(m.Content) != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
			}
			for _, call := range m.ToolCalls {
				input := call.Arguments
				if len(bytes.TrimSpace(input)) == 0 {
					input = json.RawMessage(`{}`)
				}
				blocks = append(blocks, anthropicContentBlock{Type: "tool_use", ID: call.ID, Name: call.Name, Input: input})
			}
			if len(blocks) == 0 {
				continue
			}
			messages = append(messages, anthropicMessage{Role: "assistant", Content: blocks})
		default:
			messages = append(messages, anthropicMessage{
				Role:    "user",
				Content: []anthropicContentBlock{{Type: "text", Text: m.Content}},
			})
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	payload := map[string]any{
		"model":      c.settings.ResolvedModel(),
		"max_tokens": maxTokens,
		"messages":   messages,
	}
	if strings.TrimSpace(req.System) != "" {
		payload["system"] = req.System
	}
	// Sampling parameters are not sent: recent Anthropic models reject them.
	prefilledJSON := false
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{
				"name":         tool.Name,
				"description":  tool.Description,
				"input_schema": tool.InputSchema,
			})
		}
		payload["tools"] = tools
	} else if req.JSON {
		// Anthropic has no JSON response mode, so we prefill the assistant turn
		// with an opening brace. The model then continues from "{", which both
		// suppresses prose and guarantees the reply starts a JSON object. The
		// brace is stitched back on below since the API omits the prefill.
		messages = append(messages, anthropicMessage{
			Role:    "assistant",
			Content: []anthropicContentBlock{{Type: "text", Text: "{"}},
		})
		payload["messages"] = messages
		prefilledJSON = true
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/v1/messages"), bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("x-api-key", c.settings.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic request: %w", err)
	}

	var data struct {
		Model      string                  `json:"model"`
		StopReason string                  `json:"stop_reason"`
		Content    []anthropicContentBlock `json:"content"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := decodeBody(httpResp, &data); err != nil {
		return Response{}, err
	}
	if data.StopReason == "refusal" {
		return Response{}, fmt.Errorf("anthropic declined the request (stop_reason refusal)")
	}
	// "max_tokens" means the answer was cut off; the partial text is not usable.
	if data.StopReason == "max_tokens" {
		return Response{}, ErrResponseTruncated
	}

	out := Response{Model: data.Model, Usage: Usage{InputTokens: data.Usage.InputTokens, OutputTokens: data.Usage.OutputTokens}}
	var texts []string
	for _, block := range data.Content {
		switch block.Type {
		case "text":
			texts = append(texts, block.Text)
		case "tool_use":
			out.ToolCalls = append(out.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Arguments: block.Input})
		}
	}
	out.Content = strings.TrimSpace(strings.Join(texts, "\n"))
	if prefilledJSON {
		// Re-attach the prefilled opening brace the API stripped from the reply.
		out.Content = "{" + out.Content
	}
	return out, nil
}

func (c *anthropicClient) url(path string) string {
	base := strings.TrimRight(strings.TrimSpace(c.settings.BaseURL), "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	return base + path
}
