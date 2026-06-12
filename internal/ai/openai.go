package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type openAIClient struct {
	settings Settings
	http     *http.Client
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (c *openAIClient) Complete(ctx context.Context, req Request) (Response, error) {
	messages := make([]openAIMessage, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msg := openAIMessage{Role: m.Role, Content: m.Content}
		if m.Role == RoleTool {
			msg.ToolCallID = m.ToolCallID
			msg.Name = m.ToolName
		}
		for _, call := range m.ToolCalls {
			tc := openAIToolCall{ID: call.ID, Type: "function"}
			tc.Function.Name = call.Name
			tc.Function.Arguments = string(call.Arguments)
			msg.ToolCalls = append(msg.ToolCalls, tc)
		}
		messages = append(messages, msg)
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	payload := map[string]any{
		"model":      c.settings.ResolvedModel(),
		"messages":   messages,
		"max_tokens": maxTokens,
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        tool.Name,
					"description": tool.Description,
					"parameters":  tool.InputSchema,
				},
			})
		}
		payload["tools"] = tools
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/v1/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.settings.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("openai request: %w", err)
	}

	var data struct {
		Model   string `json:"model"`
		Choices []struct {
			Message openAIMessage `json:"message"`
		} `json:"choices"`
	}
	if err := decodeBody(httpResp, &data); err != nil {
		return Response{}, err
	}
	if len(data.Choices) == 0 {
		return Response{}, fmt.Errorf("openai response missing choices")
	}

	out := Response{Content: strings.TrimSpace(data.Choices[0].Message.Content), Model: data.Model}
	for _, call := range data.Choices[0].Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
	}
	return out, nil
}

func (c *openAIClient) url(path string) string {
	base := strings.TrimRight(strings.TrimSpace(c.settings.BaseURL), "/")
	if base == "" {
		base = "https://api.openai.com"
	}
	return base + path
}
