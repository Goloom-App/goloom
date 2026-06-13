package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestSQLite_LogEntries_ComponentFilterAndSearch(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC()
	seed := []domain.LogEntry{
		{Level: "INFO", Message: "ai enhancement queued", SourceFile: "/build/internal/ai/openai.go", CreatedAt: now},
		{Level: "INFO", Message: "ai job done", SourceFile: "/build/internal/aijobs/manager.go", CreatedAt: now.Add(time.Second)},
		{Level: "WARN", Message: "mcp auth failed", SourceFile: "/build/internal/mcp/server.go", CreatedAt: now.Add(2 * time.Second)},
		{Level: "INFO", Message: "rss import started", SourceFile: "/build/internal/scheduler/rss_import.go", CreatedAt: now.Add(3 * time.Second)},
		{Level: "INFO", Message: "http server listening", SourceFile: "/build/internal/app/app.go", CreatedAt: now.Add(4 * time.Second)},
	}
	for _, e := range seed {
		if err := s.InsertLogEntry(ctx, e); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	// AI component spans both internal/ai and internal/aijobs.
	entries, err := s.ListLogEntries(ctx, domain.LogFilter{Component: domain.LogComponentAI})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("ai filter returned %d entries, want 2", len(entries))
	}
	for _, e := range entries {
		if e.Component != domain.LogComponentAI {
			t.Fatalf("entry component = %q, want ai (source %q)", e.Component, e.SourceFile)
		}
	}

	// Automation == scheduler.
	if got := countLogs(ctx, t, s, domain.LogFilter{Component: domain.LogComponentAutomation}); got != 1 {
		t.Fatalf("automation filter count = %d, want 1", got)
	}

	// System is the catch-all for everything outside the known areas (app).
	sysEntries, err := s.ListLogEntries(ctx, domain.LogFilter{Component: domain.LogComponentSystem})
	if err != nil {
		t.Fatal(err)
	}
	if len(sysEntries) != 1 || sysEntries[0].Component != domain.LogComponentSystem {
		t.Fatalf("system filter = %+v, want the single app entry", sysEntries)
	}

	// Search must not error and must match message text (regression: the WHERE
	// had two placeholders but only one bound argument).
	found, err := s.ListLogEntries(ctx, domain.LogFilter{Search: "auth failed"})
	if err != nil {
		t.Fatalf("search query errored: %v", err)
	}
	if len(found) != 1 || found[0].Component != domain.LogComponentMCP {
		t.Fatalf("search 'auth failed' = %+v, want the mcp entry", found)
	}
}

func countLogs(ctx context.Context, t *testing.T, s interface {
	CountLogEntries(context.Context, domain.LogFilter) (int, error)
}, f domain.LogFilter) int {
	t.Helper()
	n, err := s.CountLogEntries(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
