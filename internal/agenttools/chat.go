package agenttools

import (
	"context"
	"encoding/json"
	"fmt"

	"git.f4mily.net/goloom/internal/ai"
	"git.f4mily.net/goloom/internal/domain"
)

// ChatBinding is the per-conversation context the chat adapter binds into every
// tool call: the team the chat is scoped to, who is talking, and what they are
// currently looking at.
type ChatBinding struct {
	TeamID      string
	Principal   domain.AuthenticatedPrincipal
	ViewContext json.RawMessage
}

// ConfirmationRequest is emitted (as a tool payload) when the agent proposes a
// write that needs the user's explicit go-ahead. The frontend renders it as a
// confirm card and, on confirm, replays it through the confirm-action endpoint.
type ConfirmationRequest struct {
	Tool    string          `json:"tool"`
	Args    json.RawMessage `json:"args"`
	Summary string          `json:"summary"`
}

// ChatTools adapts the catalog into ai.ChatTool values for the in-app assistant.
// The team is bound from the request path (so team_id is stripped from the
// schema and injected automatically), and write tools flagged Confirm are
// proposed rather than executed.
func ChatTools(d Deps, bind ChatBinding) []ai.ChatTool {
	var out []ai.ChatTool
	for _, t := range All() {
		if !t.Exposes(TransportChat) {
			continue
		}
		tool := t
		out = append(out, ai.ChatTool{
			Tool: ai.Tool{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: stripProperty(tool.inputSchema, "team_id"),
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, json.RawMessage, error) {
				args = injectField(args, "team_id", bind.TeamID)
				if tool.Confirm {
					return proposeConfirmation(tool, args)
				}
				inv := Invocation{
					Principal:   bind.Principal,
					Transport:   TransportChat,
					ViewContext: bind.ViewContext,
				}
				res, err := tool.exec(ctx, d, inv, args)
				if err != nil {
					return "", nil, err
				}
				return res.Summary, res.Payload, nil
			},
		})
	}
	return out
}

// proposeConfirmation returns a confirmation payload instead of executing a
// write. The args carry team_id stripped back out, because the confirm-action
// endpoint re-injects it from the (re-validated) request path.
func proposeConfirmation(tool *Tool, args json.RawMessage) (string, json.RawMessage, error) {
	req := ConfirmationRequest{
		Tool:    tool.Name,
		Args:    stripField(args, "team_id"),
		Summary: fmt.Sprintf("Confirm before running %q.", tool.Name),
	}
	payload, _ := json.Marshal(map[string]any{"confirmation": req})
	return fmt.Sprintf("Proposed %q; waiting for the user to confirm before it runs.", tool.Name), payload, nil
}

// FindTool returns the catalog tool with the given name, or nil.
func FindTool(name string) *Tool {
	for _, t := range All() {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// RunConfirmed executes a tool by name over the chat transport, bypassing the
// confirmation gate. The confirm-action endpoint calls this after the user
// approves a proposed write; team access and scope are still re-checked inside
// the tool's core.
func RunConfirmed(ctx context.Context, d Deps, bind ChatBinding, name string, args json.RawMessage) (Result, error) {
	tool := FindTool(name)
	if tool == nil {
		return Result{}, fmt.Errorf("unknown tool %q", name)
	}
	if !tool.Confirm {
		return Result{}, fmt.Errorf("tool %q does not require confirmation", name)
	}
	args = injectField(args, "team_id", bind.TeamID)
	inv := Invocation{Principal: bind.Principal, Transport: TransportChat, ViewContext: bind.ViewContext}
	return tool.exec(ctx, d, inv, args)
}

// ===== JSON schema/arg helpers =====

// stripProperty removes a property from a generated object schema (both from
// "properties" and "required"), so the chat LLM never sees fields the adapter
// fills in itself (e.g. team_id, which is bound from the request path).
func stripProperty(schema json.RawMessage, name string) json.RawMessage {
	var m map[string]any
	if err := json.Unmarshal(schema, &m); err != nil {
		return schema
	}
	if props, ok := m["properties"].(map[string]any); ok {
		delete(props, name)
	}
	if req, ok := m["required"].([]any); ok {
		filtered := req[:0]
		for _, r := range req {
			if s, ok := r.(string); ok && s == name {
				continue
			}
			filtered = append(filtered, r)
		}
		if len(filtered) == 0 {
			delete(m, "required")
		} else {
			m["required"] = filtered
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return schema
	}
	return out
}

// injectField sets a top-level string field on a JSON object argument blob. Used
// to bind team_id from the request path into the chat tool's arguments.
func injectField(args json.RawMessage, name, value string) json.RawMessage {
	if value == "" {
		return args
	}
	m := map[string]json.RawMessage{}
	if len(args) > 0 {
		_ = json.Unmarshal(args, &m)
	}
	encoded, _ := json.Marshal(value)
	m[name] = encoded
	out, err := json.Marshal(m)
	if err != nil {
		return args
	}
	return out
}

// stripField removes a top-level field from a JSON object argument blob.
func stripField(args json.RawMessage, name string) json.RawMessage {
	if len(args) == 0 {
		return args
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(args, &m); err != nil {
		return args
	}
	delete(m, name)
	out, err := json.Marshal(m)
	if err != nil {
		return args
	}
	return out
}
