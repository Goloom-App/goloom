package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
)

type openAIClient struct {
	settings Settings
	http     *http.Client
	// completionTokens is set once the API rejects "max_tokens"; newer models
	// (o-series, gpt-5, …) require "max_completion_tokens" instead. Caching the
	// switch avoids a wasted round-trip on every subsequent call.
	completionTokens atomic.Bool
	// dropTemperature is set once the API rejects a custom temperature; the same
	// newer models only accept the default value, so we stop sending it.
	dropTemperature atomic.Bool
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

func (c *openAIClient) Model() string { return c.settings.ResolvedModel() }

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

	// Newer models reject two request params that older ones accept: the
	// "max_tokens" name (wants "max_completion_tokens") and any non-default
	// temperature. Each rejection is a 400 we can correct and retry, caching the
	// fix so later calls don't pay the round-trip. At most one correction per
	// rejected param, so the loop is bounded.
	var out Response
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		out, err = c.send(ctx, messages, req, maxTokens)
		if err == nil || !c.applyCorrection(err) {
			break
		}
	}
	return out, err
}

// applyCorrection inspects a failed request and, if it is a known correctable
// rejection that has not been corrected yet, flips the corresponding flag and
// reports true so the caller retries.
func (c *openAIClient) applyCorrection(err error) bool {
	if tokenParamRejected(err) && c.completionTokens.CompareAndSwap(false, true) {
		return true
	}
	if temperatureRejected(err) && c.dropTemperature.CompareAndSwap(false, true) {
		return true
	}
	return false
}

const (
	tokenParamLegacy     = "max_tokens"
	tokenParamCompletion = "max_completion_tokens"
)

func (c *openAIClient) tokenParam() string {
	if c.completionTokens.Load() {
		return tokenParamCompletion
	}
	return tokenParamLegacy
}

// tokenParamRejected reports whether err is a 400 rejecting the legacy
// "max_tokens" parameter in favour of "max_completion_tokens".
func tokenParamRejected(err error) bool {
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.status != http.StatusBadRequest {
		return false
	}
	return strings.Contains(apiErr.body, tokenParamCompletion)
}

// temperatureRejected reports whether err is a 400 rejecting a non-default
// temperature value (newer models only accept the default).
func temperatureRejected(err error) bool {
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.status != http.StatusBadRequest {
		return false
	}
	return strings.Contains(apiErr.body, "temperature") && strings.Contains(apiErr.body, "unsupported_value")
}

func (c *openAIClient) send(ctx context.Context, messages []openAIMessage, req Request, maxTokens int) (Response, error) {
	payload := map[string]any{
		"model":        c.settings.ResolvedModel(),
		"messages":     messages,
		c.tokenParam(): maxTokens,
	}
	if req.Temperature != nil && !c.dropTemperature.Load() {
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
	} else if req.JSON {
		// Constrain the model to a syntactically valid JSON object. Only valid
		// without tools, since the two response modes are mutually exclusive.
		payload["response_format"] = map[string]any{"type": "json_object"}
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
			FinishReason string        `json:"finish_reason"`
			Message      openAIMessage `json:"message"`
		} `json:"choices"`
	}
	if err := decodeBody(httpResp, &data); err != nil {
		return Response{}, err
	}
	if len(data.Choices) == 0 {
		return Response{}, fmt.Errorf("openai response missing choices")
	}
	// A "length" finish means the model ran out of room mid-answer; the body is
	// almost certainly invalid JSON, so fail loudly rather than parse garbage.
	if data.Choices[0].FinishReason == "length" {
		return Response{}, ErrResponseTruncated
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
