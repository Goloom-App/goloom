package aicontext_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/aicontext"
	"git.f4mily.net/goloom/internal/domain"
)

func TestSelectKnowledge(t *testing.T) {
	base := domain.AIContext{Team: domain.Team{ID: "team"}, KnowledgeSources: []domain.KnowledgeSource{{ID: "old", TeamID: "team", Content: "relevant"}, {ID: "new", TeamID: "team", Content: "unrelated"}, {ID: "foreign", TeamID: "other", Content: "secret"}}}
	for _, tc := range []struct {
		name, raw string
		want      []string
		fail      bool
	}{
		{"inherit", `{}`, []string{"old", "new"}, false},
		{"null inherits", `{"knowledge_source_ids":null}`, []string{"old", "new"}, false},
		{"none", `{"knowledge_source_ids":[]}`, []string{}, false},
		{"specific", `{"knowledge_source_ids":["old"]}`, []string{"old"}, false},
		{"deleted", `{"knowledge_source_ids":["missing"]}`, nil, true},
		{"foreign", `{"knowledge_source_ids":["foreign"]}`, nil, true},
		{"malformed", `{"knowledge_source_ids":"old"}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := aicontext.Select(base, json.RawMessage(tc.raw))
			if tc.fail {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, s := range got.KnowledgeSources {
				ids = append(ids, s.ID)
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("got %v want %v", ids, tc.want)
			}
		})
	}
	base.CampaignFormats = []domain.CampaignFormat{{ID: "format", TeamID: "team", KnowledgeSourceIDs: []string{"old"}}}
	got, err := aicontext.Select(base, json.RawMessage(`{"campaign_format_id":"format"}`))
	if err != nil || len(got.KnowledgeSources) != 1 || got.KnowledgeSources[0].ID != "old" {
		t.Fatalf("campaign selection: %+v %v", got, err)
	}
	_, err = aicontext.Select(base, json.RawMessage(`{"campaign_format_id":"foreign"}`))
	if !errors.Is(err, aicontext.ErrKnowledgeSourceUnavailable) {
		t.Fatalf("unknown format: %v", err)
	}
}

func TestSelectPlatformExamples(t *testing.T) {
	now := time.Now()
	base := domain.AIContext{Team: domain.Team{ID: "team"}, Accounts: []domain.AIAccountSummary{{ID: "account", Provider: "mastodon"}}, StyleExamples: []domain.StyleExample{
		{ID: "other", TeamID: "team", Platform: "bluesky", CreatedAt: now.Add(time.Hour)},
		{ID: "general", TeamID: "team", Platform: "", Content: "general", CreatedAt: now},
		{ID: "match", TeamID: "team", Platform: "mastodon", Content: "match", CreatedAt: now.Add(-time.Hour)},
		{ID: "foreign", TeamID: "other", Platform: "mastodon", CreatedAt: now},
	}}
	got, err := aicontext.Select(base, json.RawMessage(`{"target_account_ids":["account"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.StyleExamples) != 2 || got.StyleExamples[0].ID != "match" || got.StyleExamples[1].ID != "general" {
		t.Fatalf("examples: %+v", got.StyleExamples)
	}
	for _, raw := range []string{`{"platform":"mastodon"}`, `{"targetAccountIds":["account"]}`} {
		got, err := aicontext.Select(base, json.RawMessage(raw))
		if err != nil || len(got.StyleExamples) != 2 || got.StyleExamples[0].ID != "match" {
			t.Fatalf("platform alias: %+v %v", got.StyleExamples, err)
		}
	}
	got, err = aicontext.Select(base, json.RawMessage(`{"platform":"friendica"}`))
	if err != nil || len(got.StyleExamples) != 1 || got.StyleExamples[0].ID != "general" {
		t.Fatalf("general fallback: %+v %v", got.StyleExamples, err)
	}
	for i := range 4 {
		base.StyleExamples = append(base.StyleExamples, domain.StyleExample{ID: string(rune('a' + i)), TeamID: "team", Platform: "mastodon", Content: "matching", CreatedAt: now.Add(time.Duration(i) * time.Hour)})
	}
	got, err = aicontext.Select(base, json.RawMessage(`{"platform":"mastodon"}`))
	if err != nil || len(got.StyleExamples) != 3 {
		t.Fatalf("example budget: %+v %v", got.StyleExamples, err)
	}
	for _, example := range got.StyleExamples {
		if example.Platform != "mastodon" {
			t.Fatal("general displaced platform example")
		}
	}
	if base.StyleExamples[0].ID != "other" {
		t.Fatal("selection mutated source order")
	}
}
