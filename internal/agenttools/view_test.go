package agenttools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestGetCurrentView_ReturnsAttachedSnapshot(t *testing.T) {
	view := json.RawMessage(`{"section":"composer","focus":{"type":"post","id":"p1"}}`)
	out, err := coreGetCurrentView(context.Background(), Deps{}, Invocation{ViewContext: view}, GetCurrentViewInput{})
	if err != nil {
		t.Fatal(err)
	}
	if string(out.View) != string(view) {
		t.Fatalf("view = %s, want %s", out.View, view)
	}
	empty, _ := coreGetCurrentView(context.Background(), Deps{}, Invocation{}, GetCurrentViewInput{})
	if empty.Note == "" || empty.View != nil {
		t.Fatalf("empty view must return a note, got %+v", empty)
	}
}

func TestGetCurrentView_IsChatOnly(t *testing.T) {
	tool := FindTool("get_current_view")
	if tool == nil {
		t.Fatal("get_current_view missing from catalog")
	}
	if tool.Exposes(TransportMCP) {
		t.Fatal("get_current_view must not be exposed over MCP")
	}
	if !tool.Exposes(TransportChat) {
		t.Fatal("get_current_view must be exposed to chat")
	}
}

func TestViewSummary(t *testing.T) {
	if s := ViewSummary(nil); s != "" {
		t.Fatalf("nil view must summarise to empty, got %q", s)
	}
	s := ViewSummary(json.RawMessage(`{"section":"contentCalendar","focus":{"type":"post","id":"abc"}}`))
	if !strings.Contains(s, "contentCalendar") || !strings.Contains(s, "post") || !strings.Contains(s, "abc") {
		t.Fatalf("summary missing details: %q", s)
	}
}

func TestChatAdapterPassesViewContext(t *testing.T) {
	f := newFixture(t)
	view := json.RawMessage(`{"section":"composer"}`)
	tools := ChatTools(f.deps, ChatBinding{TeamID: f.team.ID, Principal: f.principal(t, `["read"]`), ViewContext: view})

	for i := range tools {
		if tools[i].Name != "get_current_view" {
			continue
		}
		summary, _, err := tools[i].Execute(context.Background(), json.RawMessage(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(summary, "composer") {
			t.Fatalf("get_current_view via chat must return the bound view, got %s", summary)
		}
		return
	}
	t.Fatal("get_current_view not exposed to chat")
}
