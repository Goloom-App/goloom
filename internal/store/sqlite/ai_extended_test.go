package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// StyleExamples
// ---------------------------------------------------------------------------

func TestSQLite_StyleExamples_CRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)

	// List on empty store
	list, err := s.ListStyleExamples(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListStyleExamples empty: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0, got %d", len(list))
	}

	// Create
	ex, err := s.CreateStyleExample(ctx, team.ID, domain.StyleExample{
		Platform: "mastodon",
		Content:  "Great post example",
		Notes:    "short and punchy",
	})
	if err != nil {
		t.Fatalf("CreateStyleExample: %v", err)
	}
	if ex.ID == "" {
		t.Fatal("expected ID")
	}
	if ex.TeamID != team.ID {
		t.Fatalf("TeamID mismatch: got %q", ex.TeamID)
	}
	if ex.Platform != "mastodon" {
		t.Fatalf("Platform: got %q", ex.Platform)
	}
	if ex.Content != "Great post example" {
		t.Fatalf("Content: got %q", ex.Content)
	}
	if ex.Notes != "short and punchy" {
		t.Fatalf("Notes: got %q", ex.Notes)
	}
	if ex.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt")
	}

	// Create a second
	ex2, err := s.CreateStyleExample(ctx, team.ID, domain.StyleExample{
		Platform: "bluesky",
		Content:  "Another example",
	})
	if err != nil {
		t.Fatalf("CreateStyleExample 2: %v", err)
	}

	// List
	list, err = s.ListStyleExamples(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListStyleExamples: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}

	// Delete first
	if err := s.DeleteStyleExample(ctx, team.ID, ex.ID); err != nil {
		t.Fatalf("DeleteStyleExample: %v", err)
	}

	// Delete second
	if err := s.DeleteStyleExample(ctx, team.ID, ex2.ID); err != nil {
		t.Fatalf("DeleteStyleExample 2: %v", err)
	}

	list, err = s.ListStyleExamples(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListStyleExamples after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list))
	}
}

func TestSQLite_StyleExamples_teamIsolation(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team1 := makeAITeam(t, s)
	_, team2 := makeAITeam(t, s)

	if _, err := s.CreateStyleExample(ctx, team1.ID, domain.StyleExample{Platform: "x", Content: "t1"}); err != nil {
		t.Fatal(err)
	}
	list2, err := s.ListStyleExamples(ctx, team2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list2) != 0 {
		t.Fatalf("team2 should see 0 examples, got %d", len(list2))
	}
}

// ---------------------------------------------------------------------------
// KnowledgeSources
// ---------------------------------------------------------------------------

func TestSQLite_KnowledgeSources_CRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)

	// Empty list
	list, err := s.ListKnowledgeSources(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListKnowledgeSources empty: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0, got %d", len(list))
	}

	// Create text source
	ks, err := s.CreateKnowledgeSource(ctx, team.ID, domain.KnowledgeSource{
		Type:    domain.KnowledgeSourceText,
		Name:    "Company bio",
		Content: "We build great software.",
	})
	if err != nil {
		t.Fatalf("CreateKnowledgeSource: %v", err)
	}
	if ks.ID == "" {
		t.Fatal("expected ID")
	}
	if ks.TeamID != team.ID {
		t.Fatalf("TeamID: got %q", ks.TeamID)
	}
	if ks.Type != domain.KnowledgeSourceText {
		t.Fatalf("Type: got %q", ks.Type)
	}
	if ks.Name != "Company bio" {
		t.Fatalf("Name: got %q", ks.Name)
	}
	if ks.Content != "We build great software." {
		t.Fatalf("Content: got %q", ks.Content)
	}

	// GetByID
	got, err := s.GetKnowledgeSourceByID(ctx, team.ID, ks.ID)
	if err != nil {
		t.Fatalf("GetKnowledgeSourceByID: %v", err)
	}
	if got.ID != ks.ID {
		t.Fatal("ID mismatch")
	}

	// GetByID - not found
	_, err = s.GetKnowledgeSourceByID(ctx, team.ID, uuid.NewString())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}

	// Create URL source
	ks2, err := s.CreateKnowledgeSource(ctx, team.ID, domain.KnowledgeSource{
		Type:      domain.KnowledgeSourceURL,
		Name:      "Docs",
		SourceURL: "https://docs.example.com",
	})
	if err != nil {
		t.Fatalf("CreateKnowledgeSource URL: %v", err)
	}

	// List
	list, err = s.ListKnowledgeSources(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListKnowledgeSources: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}

	// Update
	updated, err := s.UpdateKnowledgeSource(ctx, team.ID, ks.ID, domain.KnowledgeSource{
		Type:    domain.KnowledgeSourceText,
		Name:    "Updated bio",
		Content: "Better software.",
	})
	if err != nil {
		t.Fatalf("UpdateKnowledgeSource: %v", err)
	}
	if updated.Name != "Updated bio" {
		t.Fatalf("Name after update: got %q", updated.Name)
	}
	if updated.Content != "Better software." {
		t.Fatalf("Content after update: got %q", updated.Content)
	}

	// Delete
	if err := s.DeleteKnowledgeSource(ctx, team.ID, ks.ID); err != nil {
		t.Fatalf("DeleteKnowledgeSource: %v", err)
	}
	if err := s.DeleteKnowledgeSource(ctx, team.ID, ks2.ID); err != nil {
		t.Fatalf("DeleteKnowledgeSource 2: %v", err)
	}

	list, err = s.ListKnowledgeSources(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListKnowledgeSources after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list))
	}
}

// ---------------------------------------------------------------------------
// RSSFeedConfig
// ---------------------------------------------------------------------------

func TestSQLite_RSSFeedConfig_CRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)
	u, _ := s.UpsertOIDCUser(ctx, "rss-acc-"+uuid.NewString(), "rss@test.com", "RSS")
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://rss.test", Username: "rss", AccessToken: "at",
	})
	_ = u

	// Empty list
	list, err := s.ListRSSFeedConfigs(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListRSSFeedConfigs empty: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0, got %d", len(list))
	}

	// Create
	cfg, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{
		FeedURL:          "https://example.com/feed.xml",
		Name:             "Blog",
		IsActive:         true,
		AiEnhanceEnabled: false,
		ContentTemplate:  "{title}\n{link}",
		OutputMode:       domain.AutomationOutputDraft,
		MaxPostsPerDay:   5,
		TargetAccountIDs: []string{acc.ID},
		InitialSyncMode:  domain.RSSInitialSyncBaseline,
	})
	if err != nil {
		t.Fatalf("CreateRSSFeedConfig: %v", err)
	}
	if cfg.ID == "" {
		t.Fatal("expected ID")
	}
	if cfg.FeedURL != "https://example.com/feed.xml" {
		t.Fatalf("FeedURL: got %q", cfg.FeedURL)
	}
	if !cfg.IsActive {
		t.Fatal("expected IsActive=true")
	}
	if cfg.OutputMode != domain.AutomationOutputDraft {
		t.Fatalf("OutputMode: got %q", cfg.OutputMode)
	}
	if len(cfg.TargetAccountIDs) != 1 || cfg.TargetAccountIDs[0] != acc.ID {
		t.Fatalf("TargetAccountIDs: %v", cfg.TargetAccountIDs)
	}

	// GetByID
	got, err := s.GetRSSFeedConfigByID(ctx, team.ID, cfg.ID)
	if err != nil {
		t.Fatalf("GetRSSFeedConfigByID: %v", err)
	}
	if got.ID != cfg.ID {
		t.Fatal("ID mismatch")
	}
	if got.Name != "Blog" {
		t.Fatalf("Name: got %q", got.Name)
	}

	// GetByID - not found
	_, err = s.GetRSSFeedConfigByID(ctx, team.ID, uuid.NewString())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}

	// Create second feed
	cfg2, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{
		FeedURL:  "https://other.com/feed.xml",
		Name:     "Other",
		IsActive: false,
	})
	if err != nil {
		t.Fatalf("CreateRSSFeedConfig 2: %v", err)
	}

	// List
	list, err = s.ListRSSFeedConfigs(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListRSSFeedConfigs: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}

	// Update
	now := time.Now().UTC()
	updated, err := s.UpdateRSSFeedConfig(ctx, team.ID, cfg.ID, domain.RSSFeedConfig{
		FeedURL:        "https://example.com/feed-v2.xml",
		Name:           "Blog Updated",
		IsActive:       false,
		OutputMode:     domain.AutomationOutputScheduled,
		MaxPostsPerDay: 10,
		LastFetchedAt:  &now,
	})
	if err != nil {
		t.Fatalf("UpdateRSSFeedConfig: %v", err)
	}
	if updated.Name != "Blog Updated" {
		t.Fatalf("Name after update: got %q", updated.Name)
	}
	if updated.IsActive {
		t.Fatal("expected IsActive=false after update")
	}
	if updated.OutputMode != domain.AutomationOutputScheduled {
		t.Fatalf("OutputMode after update: got %q", updated.OutputMode)
	}
	if updated.LastFetchedAt == nil {
		t.Fatal("expected LastFetchedAt to be set")
	}

	// Delete
	if err := s.DeleteRSSFeedConfig(ctx, team.ID, cfg.ID); err != nil {
		t.Fatalf("DeleteRSSFeedConfig: %v", err)
	}
	if err := s.DeleteRSSFeedConfig(ctx, team.ID, cfg2.ID); err != nil {
		t.Fatalf("DeleteRSSFeedConfig 2: %v", err)
	}

	list, err = s.ListRSSFeedConfigs(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListRSSFeedConfigs after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list))
	}
}

// ---------------------------------------------------------------------------
// AIServiceConfig
// ---------------------------------------------------------------------------

func TestSQLite_AIServiceConfig_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)

	// Get non-existent - should return ErrNoRows
	_, err := s.GetAIServiceConfig(ctx, team.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}

	// Upsert (insert)
	cfg, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{
		Provider:    "openai",
		Model:       "gpt-4o",
		BaseURL:     "https://api.openai.com",
		APIKey:      "sk-test-key",
		Description: "Main AI config",
	})
	if err != nil {
		t.Fatalf("UpsertAIServiceConfig insert: %v", err)
	}
	if cfg.ID == "" {
		t.Fatal("expected ID")
	}
	if cfg.Provider != "openai" {
		t.Fatalf("Provider: got %q", cfg.Provider)
	}
	if cfg.Model != "gpt-4o" {
		t.Fatalf("Model: got %q", cfg.Model)
	}
	if !cfg.APIKeySet {
		t.Fatal("expected APIKeySet=true")
	}
	// APIKey is decrypted and returned from GetAIServiceConfig
	if cfg.APIKey != "sk-test-key" {
		t.Fatalf("APIKey: got %q", cfg.APIKey)
	}

	// Get
	got, err := s.GetAIServiceConfig(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetAIServiceConfig: %v", err)
	}
	if got.ID != cfg.ID {
		t.Fatal("ID mismatch")
	}
	if got.Provider != "openai" {
		t.Fatalf("Provider: got %q", got.Provider)
	}

	// Upsert (update) with new key
	cfg2, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{
		Provider:    "anthropic",
		Model:       "claude-3",
		BaseURL:     "https://api.anthropic.com",
		APIKey:      "sk-ant-key",
		Description: "Switched provider",
	})
	if err != nil {
		t.Fatalf("UpsertAIServiceConfig update: %v", err)
	}
	if cfg2.Provider != "anthropic" {
		t.Fatalf("Provider after update: got %q", cfg2.Provider)
	}
	if cfg2.APIKey != "sk-ant-key" {
		t.Fatalf("APIKey after update: got %q", cfg2.APIKey)
	}

	// Upsert (update) preserving existing key when empty APIKey given
	cfg3, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{
		Provider:    "anthropic",
		Model:       "claude-3-5",
		BaseURL:     "https://api.anthropic.com",
		APIKey:      "", // empty - should preserve existing
		Description: "Model updated",
	})
	if err != nil {
		t.Fatalf("UpsertAIServiceConfig preserve key: %v", err)
	}
	if cfg3.Model != "claude-3-5" {
		t.Fatalf("Model: got %q", cfg3.Model)
	}
	if !cfg3.APIKeySet {
		t.Fatal("expected APIKeySet preserved")
	}
}

// ---------------------------------------------------------------------------
// ProactiveTriggerSettings
// ---------------------------------------------------------------------------

func TestSQLite_ProactiveTriggerSettings_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)

	// Get non-existent - should return ErrNoRows
	_, err := s.GetProactiveTriggerSettings(ctx, team.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}

	// Upsert (insert)
	settings, err := s.UpsertProactiveTriggerSettings(ctx, team.ID, domain.ProactiveTriggerSettings{
		ContentGapThresholdDays: 3,
		AutoFillEnabled:         true,
		MaxTriggersPerDay:       5,
		CronSchedule:            "0 9 * * *",
	})
	if err != nil {
		t.Fatalf("UpsertProactiveTriggerSettings insert: %v", err)
	}
	if settings.ID == "" {
		t.Fatal("expected ID")
	}
	if settings.TeamID != team.ID {
		t.Fatalf("TeamID: got %q", settings.TeamID)
	}
	if settings.ContentGapThresholdDays != 3 {
		t.Fatalf("ContentGapThresholdDays: got %d", settings.ContentGapThresholdDays)
	}
	if !settings.AutoFillEnabled {
		t.Fatal("expected AutoFillEnabled=true")
	}
	if settings.MaxTriggersPerDay != 5 {
		t.Fatalf("MaxTriggersPerDay: got %d", settings.MaxTriggersPerDay)
	}
	if settings.CronSchedule != "0 9 * * *" {
		t.Fatalf("CronSchedule: got %q", settings.CronSchedule)
	}

	// Get
	got, err := s.GetProactiveTriggerSettings(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetProactiveTriggerSettings: %v", err)
	}
	if got.ID != settings.ID {
		t.Fatal("ID mismatch")
	}

	// Upsert (update)
	updated, err := s.UpsertProactiveTriggerSettings(ctx, team.ID, domain.ProactiveTriggerSettings{
		ContentGapThresholdDays: 7,
		AutoFillEnabled:         false,
		MaxTriggersPerDay:       2,
		CronSchedule:            "0 10 * * 1",
	})
	if err != nil {
		t.Fatalf("UpsertProactiveTriggerSettings update: %v", err)
	}
	if updated.ContentGapThresholdDays != 7 {
		t.Fatalf("ContentGapThresholdDays after update: got %d", updated.ContentGapThresholdDays)
	}
	if updated.AutoFillEnabled {
		t.Fatal("expected AutoFillEnabled=false after update")
	}
	if updated.ID != settings.ID {
		t.Fatal("ID should remain stable after upsert")
	}
}

// ---------------------------------------------------------------------------
// GetTeamAIContext
// ---------------------------------------------------------------------------

func TestSQLite_GetTeamAIContext_returnsStructure(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)

	// Should work even without profile/examples/etc
	aiCtx, err := s.GetTeamAIContext(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTeamAIContext: %v", err)
	}
	if aiCtx.Team.ID != team.ID {
		t.Fatalf("Team.ID: got %q, want %q", aiCtx.Team.ID, team.ID)
	}
	if aiCtx.Profile != nil {
		t.Fatal("expected nil profile when none created")
	}
	if len(aiCtx.CampaignFormats) != 0 {
		t.Fatalf("expected 0 CampaignFormats, got %d", len(aiCtx.CampaignFormats))
	}
	if len(aiCtx.StyleExamples) != 0 {
		t.Fatalf("expected 0 StyleExamples, got %d", len(aiCtx.StyleExamples))
	}
	if len(aiCtx.KnowledgeSources) != 0 {
		t.Fatalf("expected 0 KnowledgeSources, got %d", len(aiCtx.KnowledgeSources))
	}
}

func TestSQLite_GetTeamAIContext_includesProfileAndExamples(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)

	// Add profile
	_, err := s.CreateTeamProfile(ctx, team.ID, domain.TeamProfile{
		TeamID: team.ID,
		StyleMetadata: domain.StyleMetadata{
			Tonality: "casual",
		},
	})
	if err != nil {
		t.Fatalf("CreateTeamProfile: %v", err)
	}

	// Add style example
	if _, err := s.CreateStyleExample(ctx, team.ID, domain.StyleExample{
		Platform: "mastodon",
		Content:  "Hello world",
	}); err != nil {
		t.Fatalf("CreateStyleExample: %v", err)
	}

	// Add knowledge source
	if _, err := s.CreateKnowledgeSource(ctx, team.ID, domain.KnowledgeSource{
		Type:    domain.KnowledgeSourceText,
		Name:    "About us",
		Content: "A great company",
	}); err != nil {
		t.Fatalf("CreateKnowledgeSource: %v", err)
	}

	aiCtx, err := s.GetTeamAIContext(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTeamAIContext with profile: %v", err)
	}
	if aiCtx.Profile == nil {
		t.Fatal("expected non-nil profile")
	}
	if aiCtx.Profile.StyleMetadata.Tonality != "casual" {
		t.Fatalf("Tonality: got %q", aiCtx.Profile.StyleMetadata.Tonality)
	}
	if len(aiCtx.StyleExamples) != 1 {
		t.Fatalf("expected 1 style example, got %d", len(aiCtx.StyleExamples))
	}
	if len(aiCtx.KnowledgeSources) != 1 {
		t.Fatalf("expected 1 knowledge source, got %d", len(aiCtx.KnowledgeSources))
	}
}

// ---------------------------------------------------------------------------
// GetAIJobByIDGlobal
// ---------------------------------------------------------------------------

func TestSQLite_GetAIJobByIDGlobal(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := makeAITeam(t, s)

	job, err := s.CreateAIJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: u.ID,
		Type:         domain.AIJobTypeVoiceEngine,
		Status:       domain.AIJobStatusPending,
		Payload:      json.RawMessage(`{"prompt":"test"}`),
	})
	if err != nil {
		t.Fatalf("CreateAIJob: %v", err)
	}

	// GetAIJobByIDGlobal doesn't need teamID
	got, err := s.GetAIJobByIDGlobal(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetAIJobByIDGlobal: %v", err)
	}
	if got.ID != job.ID {
		t.Fatalf("ID: got %q, want %q", got.ID, job.ID)
	}
	if got.TeamID != team.ID {
		t.Fatalf("TeamID: got %q, want %q", got.TeamID, team.ID)
	}
	if got.Status != domain.AIJobStatusPending {
		t.Fatalf("Status: got %q", got.Status)
	}

	// Non-existent
	_, err = s.GetAIJobByIDGlobal(ctx, uuid.NewString())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}
