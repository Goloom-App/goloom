package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func newAITestTeam(t *testing.T, s interface {
	UpsertOIDCUser(ctx context.Context, subject, email, name string) (domain.User, error)
	CreateTeam(ctx context.Context, ownerUserID string, input domain.CreateTeamInput) (domain.Team, error)
}) (domain.User, domain.Team) {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "ai-"+uuid.NewString(), "ai@test.local", "AI Test")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ai-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	return u, team
}

func TestAITeamProfileCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

	input := domain.TeamProfile{
		TeamID: team.ID,
		StyleMetadata: domain.StyleMetadata{
			Tonality:          "professional",
			FormattingRules:   []string{"no emojis", "short sentences"},
			BannedWords:       []string{"synergy"},
			MaxHashtags:       3,
			PreferredLanguage: "en",
		},
		AutoPublishEnabled: false,
	}

	created, err := s.CreateTeamProfile(ctx, team.ID, input)
	if err != nil {
		t.Fatalf("CreateTeamProfile: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.TeamID != team.ID {
		t.Fatalf("TeamID: got %q, want %q", created.TeamID, team.ID)
	}
	if created.StyleMetadata.Tonality != "professional" {
		t.Fatalf("Tonality: got %q, want professional", created.StyleMetadata.Tonality)
	}
	if created.AutoPublishEnabled {
		t.Fatal("AutoPublishEnabled should be false")
	}

	got, err := s.GetTeamProfile(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTeamProfile: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("ID mismatch: got %q, want %q", got.ID, created.ID)
	}

	input.AutoPublishEnabled = true
	input.StyleMetadata.MaxHashtags = 5
	updated, err := s.UpdateTeamProfile(ctx, team.ID, input)
	if err != nil {
		t.Fatalf("UpdateTeamProfile: %v", err)
	}
	if !updated.AutoPublishEnabled {
		t.Fatal("AutoPublishEnabled should be true after update")
	}
	if updated.StyleMetadata.MaxHashtags != 5 {
		t.Fatalf("MaxHashtags: got %d, want 5", updated.StyleMetadata.MaxHashtags)
	}

	if err := s.DeleteTeamProfile(ctx, team.ID); err != nil {
		t.Fatalf("DeleteTeamProfile: %v", err)
	}

	_, err = s.GetTeamProfile(ctx, team.ID)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestAICampaignFormatCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

	weekday := 2
	structure := json.RawMessage(`{"type":"thread","posts":3}`)
	input := domain.CampaignFormat{
		TeamID:           team.ID,
		Name:             "weekly-thread-" + uuid.NewString(),
		Weekday:          &weekday,
		Structure:        structure,
		RequiredHashtags: []string{"#golang", "#tech"},
		IsActive:         true,
	}

	created, err := s.CreateCampaignFormat(ctx, team.ID, input)
	if err != nil {
		t.Fatalf("CreateCampaignFormat: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.Name != input.Name {
		t.Fatalf("Name: got %q, want %q", created.Name, input.Name)
	}
	if created.Weekday == nil || *created.Weekday != weekday {
		t.Fatalf("Weekday: got %v, want %d", created.Weekday, weekday)
	}
	if len(created.RequiredHashtags) != 2 {
		t.Fatalf("RequiredHashtags len: got %d, want 2", len(created.RequiredHashtags))
	}

	list, err := s.ListCampaignFormats(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListCampaignFormats: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListCampaignFormats: got %d, want 1", len(list))
	}

	byID, err := s.GetCampaignFormatByID(ctx, team.ID, created.ID)
	if err != nil {
		t.Fatalf("GetCampaignFormatByID: %v", err)
	}
	if byID.ID != created.ID {
		t.Fatalf("ID mismatch")
	}

	input.Name = "updated-" + uuid.NewString()
	input.IsActive = false
	updated, err := s.UpdateCampaignFormat(ctx, team.ID, created.ID, input)
	if err != nil {
		t.Fatalf("UpdateCampaignFormat: %v", err)
	}
	if updated.Name != input.Name {
		t.Fatalf("Name after update: got %q, want %q", updated.Name, input.Name)
	}
	if updated.IsActive {
		t.Fatal("IsActive should be false after update")
	}

	if err := s.DeleteCampaignFormat(ctx, team.ID, created.ID); err != nil {
		t.Fatalf("DeleteCampaignFormat: %v", err)
	}

	_, err = s.GetCampaignFormatByID(ctx, team.ID, created.ID)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestAIJobLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := newAITestTeam(t, s)

	jobInput := domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: u.ID,
		Type:         domain.AIJobTypeVoiceEngine,
		Status:       domain.AIJobStatusPending,
		Payload:      json.RawMessage(`{"prompt":"write a post"}`),
	}

	created, err := s.CreateAIJob(ctx, jobInput)
	if err != nil {
		t.Fatalf("CreateAIJob: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.Status != domain.AIJobStatusPending {
		t.Fatalf("Status: got %q, want pending", created.Status)
	}
	if created.TeamID != team.ID {
		t.Fatalf("TeamID mismatch")
	}

	byID, err := s.GetAIJobByID(ctx, team.ID, created.ID)
	if err != nil {
		t.Fatalf("GetAIJobByID: %v", err)
	}
	if byID.ID != created.ID {
		t.Fatalf("ID mismatch")
	}

	list, err := s.ListAIJobs(ctx, team.ID, 10)
	if err != nil {
		t.Fatalf("ListAIJobs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListAIJobs: got %d, want 1", len(list))
	}

	pending, err := s.ListPendingAIJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ListPendingAIJobs: %v", err)
	}
	found := false
	for _, j := range pending {
		if j.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("created job should appear in pending list")
	}

	if err := s.UpdateAIJobStatus(ctx, created.ID, domain.AIJobStatusProcessing, nil, ""); err != nil {
		t.Fatalf("UpdateAIJobStatus processing: %v", err)
	}

	result := json.RawMessage(`{"output":"hello world"}`)
	if err := s.UpdateAIJobStatus(ctx, created.ID, domain.AIJobStatusCompleted, result, ""); err != nil {
		t.Fatalf("UpdateAIJobStatus completed: %v", err)
	}

	final, err := s.GetAIJobByID(ctx, team.ID, created.ID)
	if err != nil {
		t.Fatalf("GetAIJobByID after complete: %v", err)
	}
	if final.Status != domain.AIJobStatusCompleted {
		t.Fatalf("Status: got %q, want completed", final.Status)
	}
	if final.CompletedAt == nil {
		t.Fatal("CompletedAt should be set after completion")
	}
	if len(final.Result) == 0 {
		t.Fatal("Result should be set after completion")
	}
}

func TestAIContext(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := newAITestTeam(t, s)

	profile := domain.TeamProfile{
		TeamID:             team.ID,
		StyleMetadata:      domain.StyleMetadata{Tonality: "casual", MaxHashtags: 2},
		AutoPublishEnabled: false,
	}
	if _, err := s.CreateTeamProfile(ctx, team.ID, profile); err != nil {
		t.Fatalf("CreateTeamProfile: %v", err)
	}

	for i := range 2 {
		cf := domain.CampaignFormat{
			TeamID:           team.ID,
			Name:             "format-" + uuid.NewString(),
			Structure:        json.RawMessage(`{"type":"single"}`),
			RequiredHashtags: []string{"#test"},
			IsActive:         true,
		}
		_ = i
		if _, err := s.CreateCampaignFormat(ctx, team.ID, cf); err != nil {
			t.Fatalf("CreateCampaignFormat: %v", err)
		}
	}

	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m.example", Username: "ctxu", AccessToken: "tok",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	principal := domain.AuthenticatedPrincipal{User: u}
	for range 3 {
		post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
			Content:        "test post",
			ScheduledAt:    time.Now().UTC().Add(-time.Minute),
			TargetAccounts: []string{acc.ID},
		})
		if err != nil {
			t.Fatalf("CreateScheduledPost: %v", err)
		}
		// RecentPosts only includes published posts.
		if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
			t.Fatalf("MarkPostResult: %v", err)
		}
	}

	aiCtx, err := s.GetTeamAIContext(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTeamAIContext: %v", err)
	}

	if aiCtx.Team.ID != team.ID {
		t.Fatalf("Team.ID mismatch")
	}
	if aiCtx.Profile == nil {
		t.Fatal("Profile should not be nil")
	}
	if aiCtx.Profile.StyleMetadata.Tonality != "casual" {
		t.Fatalf("Profile.Tonality: got %q, want casual", aiCtx.Profile.StyleMetadata.Tonality)
	}
	if len(aiCtx.CampaignFormats) != 2 {
		t.Fatalf("CampaignFormats: got %d, want 2", len(aiCtx.CampaignFormats))
	}
	if len(aiCtx.RecentPosts) != 3 {
		t.Fatalf("RecentPosts: got %d, want 3", len(aiCtx.RecentPosts))
	}
}

func TestAIContext_noProfile(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

	aiCtx, err := s.GetTeamAIContext(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTeamAIContext: %v", err)
	}
	if aiCtx.Profile != nil {
		t.Fatal("Profile should be nil when not created")
	}
	if aiCtx.CampaignFormats == nil {
		t.Fatal("CampaignFormats should be empty slice, not nil")
	}
}

func TestAIRSSFeedConfigCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

	input := domain.RSSFeedConfig{
		TeamID:           team.ID,
		FeedURL:          "https://blog.example/feed.xml",
		Name:             "Example Blog",
		IsActive:         true,
		PromptHint:       "Summarize this article for social media.",
		TargetAccountIDs: []string{"acct-1", "acct-2"},
		Tonality:         "professional",
	}

	created, err := s.CreateRSSFeedConfig(ctx, team.ID, input)
	if err != nil {
		t.Fatalf("CreateRSSFeedConfig: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.FeedURL != input.FeedURL {
		t.Fatalf("FeedURL: got %q, want %q", created.FeedURL, input.FeedURL)
	}
	if created.PromptHint != input.PromptHint {
		t.Fatalf("PromptHint: got %q, want %q", created.PromptHint, input.PromptHint)
	}
	if len(created.TargetAccountIDs) != 2 {
		t.Fatalf("TargetAccountIDs: got %v", created.TargetAccountIDs)
	}
	if created.Tonality != input.Tonality {
		t.Fatalf("Tonality: got %q, want %q", created.Tonality, input.Tonality)
	}

	list, err := s.ListRSSFeedConfigs(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListRSSFeedConfigs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListRSSFeedConfigs: got %d, want 1", len(list))
	}

	input.Name = "Updated Blog"
	input.IsActive = false
	fetchedAt := time.Now().UTC().Truncate(time.Second)
	input.LastFetchedAt = &fetchedAt
	updated, err := s.UpdateRSSFeedConfig(ctx, team.ID, created.ID, input)
	if err != nil {
		t.Fatalf("UpdateRSSFeedConfig: %v", err)
	}
	if updated.Name != "Updated Blog" {
		t.Fatalf("Name after update: got %q", updated.Name)
	}
	if updated.IsActive {
		t.Fatal("IsActive should be false after update")
	}
	if updated.LastFetchedAt == nil {
		t.Fatal("LastFetchedAt should be set after update")
	}

	if err := s.DeleteRSSFeedConfig(ctx, team.ID, created.ID); err != nil {
		t.Fatalf("DeleteRSSFeedConfig: %v", err)
	}

	list2, err := s.ListRSSFeedConfigs(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListRSSFeedConfigs after delete: %v", err)
	}
	if len(list2) != 0 {
		t.Fatalf("expected 0 feeds after delete, got %d", len(list2))
	}
}

func TestAIProactiveTriggerSettingsUpsert(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

	_, err := s.GetProactiveTriggerSettings(ctx, team.ID)
	if err == nil {
		t.Fatal("expected not-found error before upsert")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}

	input := domain.ProactiveTriggerSettings{
		TeamID:                  team.ID,
		ContentGapThresholdDays: 7,
		AutoFillEnabled:         true,
		MaxTriggersPerDay:       3,
		CronSchedule:            "0 10 * * *",
	}
	pts, err := s.UpsertProactiveTriggerSettings(ctx, team.ID, input)
	if err != nil {
		t.Fatalf("UpsertProactiveTriggerSettings: %v", err)
	}
	if pts.ContentGapThresholdDays != 7 {
		t.Fatalf("ContentGapThresholdDays: got %d, want 7", pts.ContentGapThresholdDays)
	}
	if !pts.AutoFillEnabled {
		t.Fatal("AutoFillEnabled should be true")
	}

	input.ContentGapThresholdDays = 14
	input.AutoFillEnabled = false
	updated, err := s.UpsertProactiveTriggerSettings(ctx, team.ID, input)
	if err != nil {
		t.Fatalf("UpsertProactiveTriggerSettings update: %v", err)
	}
	if updated.ContentGapThresholdDays != 14 {
		t.Fatalf("ContentGapThresholdDays after update: got %d, want 14", updated.ContentGapThresholdDays)
	}
	if updated.AutoFillEnabled {
		t.Fatal("AutoFillEnabled should be false after update")
	}

	got, err := s.GetProactiveTriggerSettings(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetProactiveTriggerSettings: %v", err)
	}
	if got.ContentGapThresholdDays != 14 {
		t.Fatalf("GetProactiveTriggerSettings: ContentGapThresholdDays: got %d, want 14", got.ContentGapThresholdDays)
	}
}

func TestAIServiceConfigUpsert(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

	_, err := s.GetAIServiceConfig(ctx, team.ID)
	if err == nil {
		t.Fatal("expected not-found error before upsert")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}

	cfg, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{
		Provider:    "openai",
		Model:       "gpt-4o",
		APIKey:      "secret-key-1",
		Description: "Main AI backend",
	})
	if err != nil {
		t.Fatalf("UpsertAIServiceConfig: %v", err)
	}
	if cfg.Provider != "openai" || cfg.Model != "gpt-4o" {
		t.Fatalf("provider/model: got %q/%q", cfg.Provider, cfg.Model)
	}
	if !cfg.APIKeySet || cfg.APIKey != "secret-key-1" {
		t.Fatalf("api key roundtrip: set=%v key=%q", cfg.APIKeySet, cfg.APIKey)
	}

	// Empty APIKey on update keeps the stored key.
	cfg2, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{
		Provider:    "anthropic",
		Model:       "claude-opus-4-8",
		Description: "Updated backend",
	})
	if err != nil {
		t.Fatalf("UpsertAIServiceConfig update: %v", err)
	}
	if cfg2.Provider != "anthropic" || cfg2.Model != "claude-opus-4-8" {
		t.Fatalf("provider/model after update: got %q/%q", cfg2.Provider, cfg2.Model)
	}
	if !cfg2.APIKeySet || cfg2.APIKey != "secret-key-1" {
		t.Fatalf("api key should be kept: set=%v key=%q", cfg2.APIKeySet, cfg2.APIKey)
	}

	got, err := s.GetAIServiceConfig(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetAIServiceConfig: %v", err)
	}
	if got.Provider != "anthropic" || !got.APIKeySet {
		t.Fatalf("GetAIServiceConfig: got provider %q, key set %v", got.Provider, got.APIKeySet)
	}
}
