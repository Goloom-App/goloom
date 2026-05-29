package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func newAIStoreWithRawDB(t *testing.T) (*sqlite.Store, *sql.DB) {
	t.Helper()
	enc, err := security.NewEncrypter("sqlite-integration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	s, err := sqlite.New(ctx, dsn, enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open raw DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return s, db
}

func makeAITeam(t *testing.T, s *sqlite.Store) (domain.User, domain.Team) {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "ai-user-"+uuid.NewString(), "ai@test.com", "AI Test User")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{
		Name:        "ai-team-" + uuid.NewString(),
		Description: "AI test team",
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	return u, team
}

func TestAISQLiteTeamProfileCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)

	input := domain.TeamProfile{
		TeamID: team.ID,
		StyleMetadata: domain.StyleMetadata{
			Tonality:          "professional",
			FormattingRules:   []string{"use bullets", "keep short"},
			BannedWords:       []string{"spam", "click here"},
			MaxHashtags:       5,
			PreferredLanguage: "en",
		},
		AutoPublishEnabled: true,
	}

	profile, err := s.CreateTeamProfile(ctx, team.ID, input)
	if err != nil {
		t.Fatalf("CreateTeamProfile: %v", err)
	}
	if profile.ID == "" {
		t.Fatal("expected profile.ID to be set")
	}
	if profile.TeamID != team.ID {
		t.Fatalf("TeamID: got %q, want %q", profile.TeamID, team.ID)
	}
	if !profile.AutoPublishEnabled {
		t.Fatal("expected AutoPublishEnabled=true")
	}
	if profile.StyleMetadata.Tonality != "professional" {
		t.Fatalf("Tonality: got %q", profile.StyleMetadata.Tonality)
	}

	got, err := s.GetTeamProfile(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTeamProfile: %v", err)
	}
	if got.ID != profile.ID {
		t.Fatalf("ID mismatch: got %q, want %q", got.ID, profile.ID)
	}

	input.AutoPublishEnabled = false
	input.StyleMetadata.Tonality = "casual"
	updated, err := s.UpdateTeamProfile(ctx, team.ID, input)
	if err != nil {
		t.Fatalf("UpdateTeamProfile: %v", err)
	}
	if updated.AutoPublishEnabled {
		t.Fatal("expected AutoPublishEnabled=false after update")
	}
	if updated.StyleMetadata.Tonality != "casual" {
		t.Fatalf("Tonality after update: got %q", updated.StyleMetadata.Tonality)
	}

	if err := s.DeleteTeamProfile(ctx, team.ID); err != nil {
		t.Fatalf("DeleteTeamProfile: %v", err)
	}
	_, err = s.GetTeamProfile(ctx, team.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows after delete, got %v", err)
	}
}

func TestAISQLiteCampaignFormatCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)

	weekday := 1
	input := domain.CampaignFormat{
		TeamID:           team.ID,
		Name:             "Weekly Blog Post",
		Weekday:          &weekday,
		Structure:        json.RawMessage(`{"sections": ["intro", "body", "cta"]}`),
		RequiredHashtags: []string{"#blog", "#weekly"},
		IsActive:         true,
	}

	cf, err := s.CreateCampaignFormat(ctx, team.ID, input)
	if err != nil {
		t.Fatalf("CreateCampaignFormat: %v", err)
	}
	if cf.ID == "" {
		t.Fatal("expected ID")
	}
	if cf.Name != "Weekly Blog Post" {
		t.Fatalf("Name: got %q", cf.Name)
	}
	if cf.Weekday == nil || *cf.Weekday != 1 {
		t.Fatalf("Weekday: got %v", cf.Weekday)
	}
	if len(cf.RequiredHashtags) != 2 {
		t.Fatalf("RequiredHashtags len: got %d", len(cf.RequiredHashtags))
	}
	if !cf.IsActive {
		t.Fatal("expected IsActive=true")
	}

	list, err := s.ListCampaignFormats(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListCampaignFormats: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 format, got %d", len(list))
	}

	got, err := s.GetCampaignFormatByID(ctx, team.ID, cf.ID)
	if err != nil {
		t.Fatalf("GetCampaignFormatByID: %v", err)
	}
	if got.ID != cf.ID {
		t.Fatal("ID mismatch")
	}

	input.Name = "Updated Format"
	input.IsActive = false
	updated, err := s.UpdateCampaignFormat(ctx, team.ID, cf.ID, input)
	if err != nil {
		t.Fatalf("UpdateCampaignFormat: %v", err)
	}
	if updated.Name != "Updated Format" {
		t.Fatalf("Name after update: got %q", updated.Name)
	}
	if updated.IsActive {
		t.Fatal("expected IsActive=false after update")
	}

	if err := s.DeleteCampaignFormat(ctx, team.ID, cf.ID); err != nil {
		t.Fatalf("DeleteCampaignFormat: %v", err)
	}
	list, err = s.ListCampaignFormats(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListCampaignFormats after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 formats after delete, got %d", len(list))
	}
}

func TestAISQLiteJobLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := makeAITeam(t, s)

	job, err := s.CreateAIJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: u.ID,
		Type:         domain.AIJobTypeVoiceEngine,
		Status:       domain.AIJobStatusPending,
		Payload:      json.RawMessage(`{"prompt":"write a post"}`),
	})
	if err != nil {
		t.Fatalf("CreateAIJob: %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected job.ID")
	}
	if job.Status != domain.AIJobStatusPending {
		t.Fatalf("Status: got %q", job.Status)
	}

	if err := s.UpdateAIJobStatus(ctx, job.ID, domain.AIJobStatusProcessing, nil, ""); err != nil {
		t.Fatalf("UpdateAIJobStatus processing: %v", err)
	}
	got, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatalf("GetAIJobByID: %v", err)
	}
	if got.Status != domain.AIJobStatusProcessing {
		t.Fatalf("Status after processing: got %q", got.Status)
	}
	if got.CompletedAt != nil {
		t.Fatal("expected CompletedAt=nil for processing")
	}

	result := []byte(`{"post_id":"abc123"}`)
	if err := s.UpdateAIJobStatus(ctx, job.ID, domain.AIJobStatusCompleted, result, ""); err != nil {
		t.Fatalf("UpdateAIJobStatus completed: %v", err)
	}
	completed, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatalf("GetAIJobByID completed: %v", err)
	}
	if completed.Status != domain.AIJobStatusCompleted {
		t.Fatalf("Status after completed: got %q", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set after completion")
	}
	if string(completed.Result) != `{"post_id":"abc123"}` {
		t.Fatalf("Result: got %s", string(completed.Result))
	}

	pending, err := s.ListPendingAIJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ListPendingAIJobs: %v", err)
	}
	for _, j := range pending {
		if j.ID == job.ID {
			t.Fatal("completed job should not be in pending list")
		}
	}

	all, err := s.ListAIJobs(ctx, team.ID, 10)
	if err != nil {
		t.Fatalf("ListAIJobs: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 job, got %d", len(all))
	}
}

func TestAISQLiteJsonStorage(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)

	meta := domain.StyleMetadata{
		Tonality:          "professional",
		FormattingRules:   []string{"use bullets", "keep short", "use active voice"},
		BannedWords:       []string{"spam", "click here", "amazing"},
		MaxHashtags:       7,
		PreferredLanguage: "en-US",
	}
	profile, err := s.CreateTeamProfile(ctx, team.ID, domain.TeamProfile{
		TeamID:             team.ID,
		StyleMetadata:      meta,
		AutoPublishEnabled: false,
	})
	if err != nil {
		t.Fatalf("CreateTeamProfile: %v", err)
	}

	got, err := s.GetTeamProfile(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTeamProfile: %v", err)
	}
	if got.ID != profile.ID {
		t.Fatal("ID mismatch")
	}
	if got.StyleMetadata.Tonality != meta.Tonality {
		t.Fatalf("Tonality: got %q, want %q", got.StyleMetadata.Tonality, meta.Tonality)
	}
	if len(got.StyleMetadata.FormattingRules) != len(meta.FormattingRules) {
		t.Fatalf("FormattingRules length: got %d, want %d", len(got.StyleMetadata.FormattingRules), len(meta.FormattingRules))
	}
	for i, rule := range meta.FormattingRules {
		if got.StyleMetadata.FormattingRules[i] != rule {
			t.Fatalf("FormattingRules[%d]: got %q, want %q", i, got.StyleMetadata.FormattingRules[i], rule)
		}
	}
	if len(got.StyleMetadata.BannedWords) != len(meta.BannedWords) {
		t.Fatalf("BannedWords length: got %d, want %d", len(got.StyleMetadata.BannedWords), len(meta.BannedWords))
	}
	if got.StyleMetadata.MaxHashtags != meta.MaxHashtags {
		t.Fatalf("MaxHashtags: got %d, want %d", got.StyleMetadata.MaxHashtags, meta.MaxHashtags)
	}
	if got.StyleMetadata.PreferredLanguage != meta.PreferredLanguage {
		t.Fatalf("PreferredLanguage: got %q, want %q", got.StyleMetadata.PreferredLanguage, meta.PreferredLanguage)
	}

	cf, err := s.CreateCampaignFormat(ctx, team.ID, domain.CampaignFormat{
		TeamID:           team.ID,
		Name:             "JSON Test Format",
		Structure:        json.RawMessage(`{"sections": ["intro", "body", "cta"], "max_words": 500}`),
		RequiredHashtags: []string{"#test", "#json", "#storage"},
		IsActive:         true,
	})
	if err != nil {
		t.Fatalf("CreateCampaignFormat: %v", err)
	}

	var structData struct {
		Sections []string `json:"sections"`
		MaxWords int      `json:"max_words"`
	}
	if err := json.Unmarshal(cf.Structure, &structData); err != nil {
		t.Fatalf("unmarshal structure: %v", err)
	}
	if len(structData.Sections) != 3 {
		t.Fatalf("sections length: got %d", len(structData.Sections))
	}
	if structData.MaxWords != 500 {
		t.Fatalf("max_words: got %d", structData.MaxWords)
	}
	if len(cf.RequiredHashtags) != 3 {
		t.Fatalf("RequiredHashtags length: got %d", len(cf.RequiredHashtags))
	}
}

func TestAISQLiteTeamsColumn(t *testing.T) {
	ctx := context.Background()
	s, db := newAIStoreWithRawDB(t)

	u, err := s.UpsertOIDCUser(ctx, "ai-col-"+uuid.NewString(), "aicol@test.com", "AI Col User")
	if err != nil {
		t.Fatalf("UpsertOIDCUser: %v", err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{
		Name: "ai-enabled-team-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	var enabled int
	if err := db.QueryRowContext(ctx, "select is_ai_enabled from teams where id = ?", team.ID).Scan(&enabled); err != nil {
		t.Fatalf("scan is_ai_enabled: %v", err)
	}
	if enabled != 0 {
		t.Fatalf("expected default is_ai_enabled=0, got %d", enabled)
	}

	teams, err := s.ListAIEnabledTeams(ctx)
	if err != nil {
		t.Fatalf("ListAIEnabledTeams: %v", err)
	}
	for _, tm := range teams {
		if tm.ID == team.ID {
			t.Fatal("team should not be in AI-enabled list before enabling")
		}
	}

	if _, err := db.ExecContext(ctx, "update teams set is_ai_enabled = 1 where id = ?", team.ID); err != nil {
		t.Fatalf("update is_ai_enabled: %v", err)
	}

	teams, err = s.ListAIEnabledTeams(ctx)
	if err != nil {
		t.Fatalf("ListAIEnabledTeams after enable: %v", err)
	}
	var found bool
	for _, tm := range teams {
		if tm.ID == team.ID {
			found = true
			if !tm.IsAIEnabled {
				t.Fatal("expected IsAIEnabled=true")
			}
			break
		}
	}
	if !found {
		t.Fatal("expected team in AI-enabled list after setting is_ai_enabled=1")
	}
}
