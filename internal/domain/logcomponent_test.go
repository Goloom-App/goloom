package domain

import "testing"

func TestLogComponentFromSource(t *testing.T) {
	cases := map[string]string{
		"/home/dev/git/goloom/internal/ai/openai.go":         LogComponentAI,
		"/app/internal/aijobs/manager.go":                    LogComponentAI,
		"/build/internal/mcp/server.go":                      LogComponentMCP,
		"/home/dev/git/goloom/internal/scheduler/rss_ai.go":  LogComponentAutomation,
		"/x/internal/provider/mastodon/client.go":            LogComponentProvider,
		"/home/dev/git/goloom/api/http.go":                   LogComponentAPI,
		"/home/dev/git/goloom/internal/store/sqlite/logs.go": LogComponentSystem,
		"": LogComponentSystem,
	}
	for source, want := range cases {
		if got := LogComponentFromSource(source); got != want {
			t.Errorf("LogComponentFromSource(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestLogComponentFilter(t *testing.T) {
	frags, negate := LogComponentFilter(LogComponentAI)
	if negate || len(frags) != 2 {
		t.Fatalf("ai filter = %v negate=%v, want 2 fragments positive", frags, negate)
	}

	all, negate := LogComponentFilter(LogComponentSystem)
	if !negate || len(all) == 0 {
		t.Fatalf("system filter should be negated with all fragments, got %v negate=%v", all, negate)
	}

	if frags, _ := LogComponentFilter("nonsense"); frags != nil {
		t.Fatalf("unknown component should impose no constraint, got %v", frags)
	}
}

func TestLogComponentsListed(t *testing.T) {
	comps := LogComponents()
	if len(comps) == 0 || comps[len(comps)-1] != LogComponentSystem {
		t.Fatalf("LogComponents should end with system bucket: %v", comps)
	}
}
