package agenttools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestCatalogNoDuplicateNames guards the single-source-of-truth invariant: a
// tool name must be unique across the catalog, or one definition silently
// shadows the other on a transport.
func TestCatalogNoDuplicateNames(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range All() {
		if seen[tool.Name] {
			t.Fatalf("duplicate tool name %q in catalog", tool.Name)
		}
		seen[tool.Name] = true
	}
}

// TestEverySchemaIsValidObject ensures schema generation produced a usable JSON
// schema object for every tool (the chat LLM and the MCP SDK both need one).
func TestEverySchemaIsValidObject(t *testing.T) {
	for _, tool := range All() {
		var m map[string]any
		if err := json.Unmarshal(tool.inputSchema, &m); err != nil {
			t.Fatalf("tool %q: schema is not valid JSON: %v", tool.Name, err)
		}
		if m["type"] != "object" {
			t.Fatalf("tool %q: schema type = %v, want object", tool.Name, m["type"])
		}
	}
}

// TestReadToolsSharedAcrossTransports is the parity guard: the read/insight
// tools must be exposed on BOTH the MCP server and the chat assistant so the two
// surfaces cannot drift apart.
func TestReadToolsSharedAcrossTransports(t *testing.T) {
	shared := map[string]bool{
		"get_calendar": true, "find_free_slot": true, "get_posts": true,
		"search_posts": true, "get_campaign": true, "get_platforms": true,
		"get_brand_profile": true, "get_analytics": true,
		"get_hashtag_performance": true, "get_analytics_timeslots": true,
		"get_account_growth": true, "get_metric_history": true,
	}
	for _, tool := range All() {
		if !shared[tool.Name] {
			continue
		}
		if !tool.Exposes(TransportMCP) || !tool.Exposes(TransportChat) {
			t.Fatalf("read tool %q must be exposed on both transports, got %v", tool.Name, tool.Transports)
		}
		delete(shared, tool.Name)
	}
	if len(shared) != 0 {
		t.Fatalf("expected shared read tools missing from catalog: %v", shared)
	}
}

// TestChatSchemasOmitTeamID checks that the chat adapter strips team_id (bound
// from the request path) from every chat tool's schema, so the LLM never has to
// supply it.
func TestChatSchemasOmitTeamID(t *testing.T) {
	f := newFixture(t)
	tools := ChatTools(f.deps, ChatBinding{TeamID: f.team.ID, Principal: f.principal(t, `["read"]`)})
	if len(tools) == 0 {
		t.Fatal("expected chat tools")
	}
	for _, tool := range tools {
		var m map[string]any
		if err := json.Unmarshal(tool.InputSchema, &m); err != nil {
			t.Fatalf("tool %q: %v", tool.Name, err)
		}
		if props, ok := m["properties"].(map[string]any); ok {
			if _, has := props["team_id"]; has {
				t.Fatalf("chat tool %q schema must not expose team_id", tool.Name)
			}
		}
	}
}

// TestChatAdapterBindsTeamAndRuns drives a read tool through the chat adapter
// exactly as the assistant would: empty args, team bound from the path.
func TestChatAdapterBindsTeamAndRuns(t *testing.T) {
	f := newFixture(t)
	tools := ChatTools(f.deps, ChatBinding{TeamID: f.team.ID, Principal: f.principal(t, `["read"]`)})

	var platforms *toolRunner
	for i := range tools {
		if tools[i].Name == "get_platforms" {
			platforms = &toolRunner{exec: tools[i].Execute}
		}
	}
	if platforms == nil {
		t.Fatal("get_platforms not exposed to chat")
	}
	summary, _, err := platforms.exec(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("get_platforms via chat: %v", err)
	}
	if !strings.Contains(summary, f.account.ID) {
		t.Fatalf("expected account %s in summary, got %s", f.account.ID, summary)
	}
}

type toolRunner struct {
	exec func(context.Context, json.RawMessage) (string, json.RawMessage, error)
}
