package postgres_test

import (
	"context"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestListActiveRSSFeedConfigs_scansAllColumns(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

	active, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{
		TeamID:           team.ID,
		FeedURL:          "https://blog.example/active.xml",
		Name:             "Active Feed",
		IsActive:         true,
		AiEnhanceEnabled: true,
		TargetAccountIDs: []string{"acct-1"},
		ContentTemplate:  "{title}",
		OutputMode:       domain.AutomationOutputDraft,
	})
	if err != nil {
		t.Fatalf("CreateRSSFeedConfig active: %v", err)
	}

	_, err = s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{
		TeamID:           team.ID,
		FeedURL:          "https://blog.example/inactive.xml",
		Name:             "Inactive Feed",
		IsActive:         false,
		TargetAccountIDs: []string{"acct-1"},
	})
	if err != nil {
		t.Fatalf("CreateRSSFeedConfig inactive: %v", err)
	}

	feeds, err := s.ListActiveRSSFeedConfigs(ctx, 10)
	if err != nil {
		t.Fatalf("ListActiveRSSFeedConfigs: %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("ListActiveRSSFeedConfigs: got %d feeds, want 1", len(feeds))
	}
	got := feeds[0]
	if got.ID != active.ID {
		t.Fatalf("feed ID: got %q, want %q", got.ID, active.ID)
	}
	if !got.AiEnhanceEnabled {
		t.Fatal("expected ai_enhance_enabled=true to scan correctly")
	}
	if got.ContentTemplate == "" {
		t.Fatal("expected content_template to scan correctly")
	}
}
