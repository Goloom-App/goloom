package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func makePostTemplateFixture(t *testing.T) (context.Context, interface {
	CreateAccount(ctx context.Context, teamID string, input domain.ConnectedAccount) (domain.SocialAccount, error)
	CreateTeam(ctx context.Context, ownerUserID string, input domain.CreateTeamInput) (domain.Team, error)
	UpsertOIDCUser(ctx context.Context, subject, email, name string) (domain.User, error)
	CreatePostTemplate(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostTemplateInput) (domain.PostTemplate, error)
	GetPostTemplate(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error)
	ListEnabledPostTemplates(ctx context.Context, limit int) ([]domain.PostTemplate, error)
	ListDuePostTemplates(ctx context.Context, limit int) ([]domain.PostTemplate, error)
	UpdatePostTemplate(ctx context.Context, teamID, templateID string, input domain.UpdatePostTemplateInput) (domain.PostTemplate, error)
	DeletePostTemplate(ctx context.Context, teamID, templateID string) error
	IsPostTemplateOccurrenceSkipped(ctx context.Context, templateID string, occurrenceAt time.Time) (bool, error)
	IsPostTemplateAnnouncementSkipped(ctx context.Context, templateID string, occurrenceAt time.Time) (bool, error)
	AddPostTemplateSkip(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) error
	AddPostTemplateAnnouncementSkip(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) error
}, domain.Team, domain.SocialAccount, domain.AuthenticatedPrincipal) {
	t.Helper()
	ctx := context.Background()
	s := newTestStore(t)
	u, err := s.UpsertOIDCUser(ctx, "pt-"+uuid.NewString(), "pt@test.com", "PT User")
	if err != nil {
		t.Fatalf("UpsertOIDCUser: %v", err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{
		Name:        "pt-team-" + uuid.NewString(),
		Description: "post template test team",
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://pt.test", Username: "pt", AccessToken: "at",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}
	return ctx, s, team, acc, principal
}

const weeklyRecurrence = `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`

func TestSQLite_PostTemplate_CreateAndGet(t *testing.T) {
	ctx, s, team, acc, principal := makePostTemplateFixture(t)

	enabled := true
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:            "Weekly Update",
		Content:          "This week: {counter}",
		RecurrenceJSON:   weeklyRecurrence,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}
	if tmpl.ID == "" {
		t.Fatal("expected ID")
	}
	if tmpl.TeamID != team.ID {
		t.Fatalf("TeamID: got %q", tmpl.TeamID)
	}
	if tmpl.Title != "Weekly Update" {
		t.Fatalf("Title: got %q", tmpl.Title)
	}
	if tmpl.Content != "This week: {counter}" {
		t.Fatalf("Content: got %q", tmpl.Content)
	}
	if !tmpl.Enabled {
		t.Fatal("expected Enabled=true")
	}
	if len(tmpl.TargetAccountIDs) != 1 || tmpl.TargetAccountIDs[0] != acc.ID {
		t.Fatalf("TargetAccountIDs: %v", tmpl.TargetAccountIDs)
	}
	if tmpl.NextMaterializeAt == nil {
		t.Fatal("expected NextMaterializeAt to be set for enabled template")
	}
	if tmpl.CounterNext != 1 {
		t.Fatalf("CounterNext: got %d, want 1", tmpl.CounterNext)
	}

	// GetPostTemplate
	got, err := s.GetPostTemplate(ctx, team.ID, tmpl.ID)
	if err != nil {
		t.Fatalf("GetPostTemplate: %v", err)
	}
	if got.ID != tmpl.ID {
		t.Fatal("ID mismatch")
	}
	if got.Title != tmpl.Title {
		t.Fatalf("Title: got %q", got.Title)
	}
}

func TestSQLite_PostTemplate_ListEnabledAndDue(t *testing.T) {
	ctx, s, team, acc, principal := makePostTemplateFixture(t)

	enabled := true
	disabled := false

	// Create an enabled template
	tmpl1, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:            "Enabled Template",
		Content:          "Recurring content",
		RecurrenceJSON:   weeklyRecurrence,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate enabled: %v", err)
	}

	// Create a disabled template
	if _, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:            "Disabled Template",
		Content:          "Not scheduled",
		RecurrenceJSON:   weeklyRecurrence,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &disabled,
	}); err != nil {
		t.Fatalf("CreatePostTemplate disabled: %v", err)
	}

	// ListEnabledPostTemplates should only return enabled ones with next_materialize_at set
	list, err := s.ListEnabledPostTemplates(ctx, 50)
	if err != nil {
		t.Fatalf("ListEnabledPostTemplates: %v", err)
	}
	found := false
	for _, pt := range list {
		if pt.ID == tmpl1.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("enabled template not in ListEnabledPostTemplates")
	}

	// ListDuePostTemplates is equivalent to ListEnabledPostTemplates
	due, err := s.ListDuePostTemplates(ctx, 50)
	if err != nil {
		t.Fatalf("ListDuePostTemplates: %v", err)
	}
	foundDue := false
	for _, pt := range due {
		if pt.ID == tmpl1.ID {
			foundDue = true
		}
	}
	if !foundDue {
		t.Fatal("enabled template not in ListDuePostTemplates")
	}
}

func TestSQLite_PostTemplate_Update(t *testing.T) {
	ctx, s, team, acc, principal := makePostTemplateFixture(t)

	enabled := true
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:            "Original",
		Content:          "Original content",
		RecurrenceJSON:   weeklyRecurrence,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}

	// Update title and content
	newTitle := "Updated Title"
	newContent := "Updated content"
	updated, err := s.UpdatePostTemplate(ctx, team.ID, tmpl.ID, domain.UpdatePostTemplateInput{
		Title:   &newTitle,
		Content: &newContent,
	})
	if err != nil {
		t.Fatalf("UpdatePostTemplate: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Fatalf("Title after update: got %q", updated.Title)
	}
	if updated.Content != "Updated content" {
		t.Fatalf("Content after update: got %q", updated.Content)
	}

	// Disable via update
	disabled := false
	updated2, err := s.UpdatePostTemplate(ctx, team.ID, tmpl.ID, domain.UpdatePostTemplateInput{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("UpdatePostTemplate disable: %v", err)
	}
	if updated2.Enabled {
		t.Fatal("expected Enabled=false after update")
	}
	// When disabled, next_materialize_at should be nil
	if updated2.NextMaterializeAt != nil {
		t.Fatal("expected NextMaterializeAt=nil for disabled template")
	}
}

func TestSQLite_PostTemplate_Delete(t *testing.T) {
	ctx, s, team, acc, principal := makePostTemplateFixture(t)

	enabled := true
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:            "To Delete",
		Content:          "gone soon",
		RecurrenceJSON:   weeklyRecurrence,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}

	// Delete
	if err := s.DeletePostTemplate(ctx, team.ID, tmpl.ID); err != nil {
		t.Fatalf("DeletePostTemplate: %v", err)
	}

	// Delete non-existent
	if err := s.DeletePostTemplate(ctx, team.ID, tmpl.ID); err == nil {
		t.Fatal("expected error deleting non-existent template")
	}
}

func TestSQLite_PostTemplate_OccurrenceSkip(t *testing.T) {
	ctx, s, team, acc, principal := makePostTemplateFixture(t)

	enabled := true
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:            "Skip Test",
		Content:          "occurrence",
		RecurrenceJSON:   weeklyRecurrence,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}

	occAt := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)

	// Not skipped initially
	skipped, err := s.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, occAt)
	if err != nil {
		t.Fatalf("IsPostTemplateOccurrenceSkipped: %v", err)
	}
	if skipped {
		t.Fatal("expected not skipped initially")
	}

	// Add skip
	if err := s.AddPostTemplateSkip(ctx, team.ID, tmpl.ID, occAt); err != nil {
		t.Fatalf("AddPostTemplateSkip: %v", err)
	}

	// Now skipped
	skipped, err = s.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, occAt)
	if err != nil {
		t.Fatalf("IsPostTemplateOccurrenceSkipped after skip: %v", err)
	}
	if !skipped {
		t.Fatal("expected skipped after AddPostTemplateSkip")
	}

	// Different occurrence - not skipped
	other := occAt.Add(7 * 24 * time.Hour)
	otherSkipped, err := s.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, other)
	if err != nil {
		t.Fatalf("IsPostTemplateOccurrenceSkipped other: %v", err)
	}
	if otherSkipped {
		t.Fatal("different occurrence should not be skipped")
	}

	// AddPostTemplateSkip on non-existent template should fail
	if err := s.AddPostTemplateSkip(ctx, team.ID, uuid.NewString(), occAt); err == nil {
		t.Fatal("expected error skipping non-existent template")
	}
}

func TestSQLite_PostTemplate_AnnouncementSkip(t *testing.T) {
	ctx, s, team, acc, principal := makePostTemplateFixture(t)

	enabled := true
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:            "Ann Skip Test",
		Content:          "announcement",
		RecurrenceJSON:   weeklyRecurrence,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}

	occAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	// Not announcement-skipped initially
	skipped, err := s.IsPostTemplateAnnouncementSkipped(ctx, tmpl.ID, occAt)
	if err != nil {
		t.Fatalf("IsPostTemplateAnnouncementSkipped: %v", err)
	}
	if skipped {
		t.Fatal("expected not announcement-skipped initially")
	}

	// A skipped occurrence must not be announced either: the occurrence skip
	// implies the announcement skip (matches the postgres store semantics).
	if err := s.AddPostTemplateSkip(ctx, team.ID, tmpl.ID, occAt); err != nil {
		t.Fatalf("AddPostTemplateSkip: %v", err)
	}
	annSkipped, err := s.IsPostTemplateAnnouncementSkipped(ctx, tmpl.ID, occAt)
	if err != nil {
		t.Fatalf("IsPostTemplateAnnouncementSkipped after occurrence skip: %v", err)
	}
	if !annSkipped {
		t.Fatal("occurrence skip must imply announcement skip")
	}
}

func TestSQLite_PostTemplate_OccurrenceSkipUpgradesAnnouncementSkip(t *testing.T) {
	ctx, s, team, acc, principal := makePostTemplateFixture(t)

	enabled := true
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:            "Skip Upgrade Test",
		Content:          "announcement",
		RecurrenceJSON:   weeklyRecurrence,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}
	occAt := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)

	if err := s.AddPostTemplateAnnouncementSkip(ctx, team.ID, tmpl.ID, occAt); err != nil {
		t.Fatalf("AddPostTemplateAnnouncementSkip: %v", err)
	}
	if err := s.AddPostTemplateSkip(ctx, team.ID, tmpl.ID, occAt); err != nil {
		t.Fatalf("AddPostTemplateSkip after announcement skip: %v", err)
	}
	skipped, err := s.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, occAt)
	if err != nil {
		t.Fatalf("IsPostTemplateOccurrenceSkipped: %v", err)
	}
	if !skipped {
		t.Fatal("occurrence skip must replace announcement-only skip")
	}
}

func TestSQLite_PostTemplate_WithOutputModeAndHints(t *testing.T) {
	ctx, s, team, acc, principal := makePostTemplateFixture(t)

	enabled := true
	horizon := 7
	counterStart := 5
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:                  "Advanced Template",
		Content:                "Draft post #{counter}",
		RecurrenceJSON:         weeklyRecurrence,
		TargetAccountIDs:       []string{acc.ID},
		Enabled:                &enabled,
		OutputMode:             domain.AutomationOutputDraft,
		PromptHint:             "write something good",
		TitleHint:              "episode title",
		Tonality:               "casual",
		MaterializeHorizonDays: &horizon,
		CounterNext:            &counterStart,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}
	if tmpl.OutputMode != domain.AutomationOutputDraft {
		t.Fatalf("OutputMode: got %q", tmpl.OutputMode)
	}
	if tmpl.PromptHint != "write something good" {
		t.Fatalf("PromptHint: got %q", tmpl.PromptHint)
	}
	if tmpl.TitleHint != "episode title" {
		t.Fatalf("TitleHint: got %q", tmpl.TitleHint)
	}
	if tmpl.Tonality != "casual" {
		t.Fatalf("Tonality: got %q", tmpl.Tonality)
	}
	if tmpl.MaterializeHorizonDays != 7 {
		t.Fatalf("MaterializeHorizonDays: got %d", tmpl.MaterializeHorizonDays)
	}
	if tmpl.CounterNext != 5 {
		t.Fatalf("CounterNext: got %d", tmpl.CounterNext)
	}
}
