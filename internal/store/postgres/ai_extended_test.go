package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// StyleExamples
// ---------------------------------------------------------------------------

func TestPostgres_StyleExamples_CRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

	// Empty
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
		Content:  "Example content for postgres",
		Notes:    "some notes",
	})
	if err != nil {
		t.Fatalf("CreateStyleExample: %v", err)
	}
	if ex.ID == "" {
		t.Fatal("expected ID")
	}
	if ex.TeamID != team.ID {
		t.Fatalf("TeamID: got %q", ex.TeamID)
	}
	if ex.Platform != "mastodon" {
		t.Fatalf("Platform: got %q", ex.Platform)
	}
	if ex.Content != "Example content for postgres" {
		t.Fatalf("Content: got %q", ex.Content)
	}

	// Second example
	ex2, err := s.CreateStyleExample(ctx, team.ID, domain.StyleExample{
		Platform: "bluesky",
		Content:  "Another one",
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

	// Delete
	if err := s.DeleteStyleExample(ctx, team.ID, ex.ID); err != nil {
		t.Fatalf("DeleteStyleExample: %v", err)
	}
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

// ---------------------------------------------------------------------------
// KnowledgeSources
// ---------------------------------------------------------------------------

func TestPostgres_KnowledgeSources_CRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

	// Empty
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
		Name:    "Company overview",
		Content: "We build software.",
	})
	if err != nil {
		t.Fatalf("CreateKnowledgeSource: %v", err)
	}
	if ks.ID == "" {
		t.Fatal("expected ID")
	}
	if ks.Type != domain.KnowledgeSourceText {
		t.Fatalf("Type: got %q", ks.Type)
	}

	// GetByID
	got, err := s.GetKnowledgeSourceByID(ctx, team.ID, ks.ID)
	if err != nil {
		t.Fatalf("GetKnowledgeSourceByID: %v", err)
	}
	if got.ID != ks.ID {
		t.Fatal("ID mismatch")
	}

	// GetByID - not found (pgx returns pgx.ErrNoRows)
	_, err = s.GetKnowledgeSourceByID(ctx, team.ID, uuid.NewString())
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}

	// Create URL source
	ks2, err := s.CreateKnowledgeSource(ctx, team.ID, domain.KnowledgeSource{
		Type:      domain.KnowledgeSourceURL,
		Name:      "Docs link",
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
		Name:    "Updated overview",
		Content: "Better software.",
	})
	if err != nil {
		t.Fatalf("UpdateKnowledgeSource: %v", err)
	}
	if updated.Name != "Updated overview" {
		t.Fatalf("Name after update: got %q", updated.Name)
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
// GetAIJobByIDGlobal
// ---------------------------------------------------------------------------

func TestPostgres_GetAIJobByIDGlobal(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := newAITestTeam(t, s)

	job, err := s.CreateAIJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: u.ID,
		Type:         domain.AIJobTypeVoiceEngine,
		Status:       domain.AIJobStatusPending,
	})
	if err != nil {
		t.Fatalf("CreateAIJob: %v", err)
	}

	// GetAIJobByIDGlobal without teamID
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

	// Non-existent
	_, err = s.GetAIJobByIDGlobal(ctx, uuid.NewString())
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// PostTemplates
// ---------------------------------------------------------------------------

const pgWeeklyRecurrence = `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`

func TestPostgres_PostTemplate_CRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "pt-pg-"+uuid.NewString(), "pt@pg.test", "PT PG")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "pt-" + uuid.NewString()})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://pt-pg.test", Username: "pt", AccessToken: "at",
	})
	principal := domain.AuthenticatedPrincipal{User: u}

	enabled := true
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:            "PG Weekly",
		Content:          "This week: {counter}",
		RecurrenceJSON:   pgWeeklyRecurrence,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}
	if tmpl.ID == "" {
		t.Fatal("expected ID")
	}
	if tmpl.Title != "PG Weekly" {
		t.Fatalf("Title: got %q", tmpl.Title)
	}
	if !tmpl.Enabled {
		t.Fatal("expected Enabled=true")
	}
	if tmpl.NextMaterializeAt == nil {
		t.Fatal("expected NextMaterializeAt set for enabled template")
	}

	// GetPostTemplate
	got, err := s.GetPostTemplate(ctx, team.ID, tmpl.ID)
	if err != nil {
		t.Fatalf("GetPostTemplate: %v", err)
	}
	if got.ID != tmpl.ID {
		t.Fatal("ID mismatch")
	}

	// ListEnabledPostTemplates / ListDuePostTemplates
	list, err := s.ListEnabledPostTemplates(ctx, 50)
	if err != nil {
		t.Fatalf("ListEnabledPostTemplates: %v", err)
	}
	var found bool
	for _, pt := range list {
		if pt.ID == tmpl.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("enabled template not in ListEnabledPostTemplates")
	}

	due, err := s.ListDuePostTemplates(ctx, 50)
	if err != nil {
		t.Fatalf("ListDuePostTemplates: %v", err)
	}
	var foundDue bool
	for _, pt := range due {
		if pt.ID == tmpl.ID {
			foundDue = true
		}
	}
	if !foundDue {
		t.Fatal("enabled template not in ListDuePostTemplates")
	}

	// UpdatePostTemplate
	newContent := "Updated {counter}"
	updated, err := s.UpdatePostTemplate(ctx, team.ID, tmpl.ID, domain.UpdatePostTemplateInput{
		Content: &newContent,
	})
	if err != nil {
		t.Fatalf("UpdatePostTemplate: %v", err)
	}
	if updated.Content != "Updated {counter}" {
		t.Fatalf("Content after update: got %q", updated.Content)
	}

	// Disable
	disabled := false
	updated2, err := s.UpdatePostTemplate(ctx, team.ID, tmpl.ID, domain.UpdatePostTemplateInput{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("UpdatePostTemplate disable: %v", err)
	}
	if updated2.Enabled {
		t.Fatal("expected Enabled=false")
	}
	if updated2.NextMaterializeAt != nil {
		t.Fatal("expected NextMaterializeAt=nil for disabled template")
	}

	// OccurrenceSkip
	occAt := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)

	skipped, err := s.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, occAt)
	if err != nil {
		t.Fatalf("IsPostTemplateOccurrenceSkipped: %v", err)
	}
	if skipped {
		t.Fatal("expected not skipped initially")
	}

	if err := s.AddPostTemplateSkip(ctx, team.ID, tmpl.ID, occAt); err != nil {
		t.Fatalf("AddPostTemplateSkip: %v", err)
	}

	skipped, err = s.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, occAt)
	if err != nil {
		t.Fatalf("IsPostTemplateOccurrenceSkipped after skip: %v", err)
	}
	if !skipped {
		t.Fatal("expected skipped after AddPostTemplateSkip")
	}

	// A skipped occurrence must not be announced either: the occurrence skip
	// implies the announcement skip.
	annSkipped, err := s.IsPostTemplateAnnouncementSkipped(ctx, tmpl.ID, occAt)
	if err != nil {
		t.Fatalf("IsPostTemplateAnnouncementSkipped: %v", err)
	}
	if !annSkipped {
		t.Fatal("occurrence skip must imply announcement skip")
	}

	occAfterAnnouncementSkip := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	if err := s.AddPostTemplateAnnouncementSkip(ctx, team.ID, tmpl.ID, occAfterAnnouncementSkip); err != nil {
		t.Fatalf("AddPostTemplateAnnouncementSkip: %v", err)
	}
	if err := s.AddPostTemplateSkip(ctx, team.ID, tmpl.ID, occAfterAnnouncementSkip); err != nil {
		t.Fatalf("AddPostTemplateSkip after announcement skip: %v", err)
	}
	skipped, err = s.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, occAfterAnnouncementSkip)
	if err != nil {
		t.Fatalf("IsPostTemplateOccurrenceSkipped after upgrade: %v", err)
	}
	if !skipped {
		t.Fatal("occurrence skip must replace announcement-only skip")
	}

	// DeletePostTemplate
	if err := s.DeletePostTemplate(ctx, team.ID, tmpl.ID); err != nil {
		t.Fatalf("DeletePostTemplate: %v", err)
	}

	// Delete non-existent should fail
	if err := s.DeletePostTemplate(ctx, team.ID, tmpl.ID); err == nil {
		t.Fatal("expected error deleting non-existent template")
	}
}

// ---------------------------------------------------------------------------
// Logs (ArchiveLogEntry, UnarchiveLogEntry, DeleteLogEntry, DeleteLogEntriesBefore)
// ---------------------------------------------------------------------------

func TestPostgres_LogEntry_ArchiveUnarchiveDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Insert a log entry
	id := uuid.NewString()
	now := time.Now().UTC()
	if err := s.InsertLogEntry(ctx, domain.LogEntry{
		ID:         id,
		Level:      "info",
		Message:    "test pg log",
		SourceFile: "pkg/test.go",
		SourceLine: 1,
		CreatedAt:  now,
		Attributes: map[string]string{},
	}); err != nil {
		t.Fatalf("InsertLogEntry: %v", err)
	}

	// Archive
	if err := s.ArchiveLogEntry(ctx, id); err != nil {
		t.Fatalf("ArchiveLogEntry: %v", err)
	}

	// Should not appear in non-archived filter
	archivedBool := false
	notArchived, err := s.ListLogEntries(ctx, domain.LogFilter{Limit: 500, Archived: &archivedBool})
	if err != nil {
		t.Fatalf("ListLogEntries: %v", err)
	}
	for _, e := range notArchived {
		if e.ID == id {
			t.Fatal("archived entry should not appear in archived=false filter")
		}
	}

	// Should appear in archived filter
	archivedTrue := true
	archivedList, err := s.ListLogEntries(ctx, domain.LogFilter{Limit: 500, Archived: &archivedTrue})
	if err != nil {
		t.Fatalf("ListLogEntries archived: %v", err)
	}
	var foundArchived bool
	for _, e := range archivedList {
		if e.ID == id {
			foundArchived = true
			if e.ArchivedAt == nil {
				t.Fatal("expected ArchivedAt set")
			}
		}
	}
	if !foundArchived {
		t.Fatal("archived entry not found in archived=true filter")
	}

	// Unarchive
	if err := s.UnarchiveLogEntry(ctx, id); err != nil {
		t.Fatalf("UnarchiveLogEntry: %v", err)
	}

	// Delete
	if err := s.DeleteLogEntry(ctx, id); err != nil {
		t.Fatalf("DeleteLogEntry: %v", err)
	}

	// Should be gone
	all, err := s.ListLogEntries(ctx, domain.LogFilter{Limit: 500})
	if err != nil {
		t.Fatalf("ListLogEntries after delete: %v", err)
	}
	for _, e := range all {
		if e.ID == id {
			t.Fatal("deleted entry still present")
		}
	}
}

func TestPostgres_DeleteLogEntriesBefore(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	old := time.Now().UTC().Add(-48 * time.Hour)
	idOld := uuid.NewString()
	if err := s.InsertLogEntry(ctx, domain.LogEntry{
		ID:         idOld,
		Level:      "info",
		Message:    "old pg log",
		SourceFile: "pkg/old.go",
		SourceLine: 1,
		CreatedAt:  old,
		Attributes: map[string]string{},
	}); err != nil {
		t.Fatalf("InsertLogEntry old: %v", err)
	}

	idRecent := uuid.NewString()
	if err := s.InsertLogEntry(ctx, domain.LogEntry{
		ID:         idRecent,
		Level:      "info",
		Message:    "recent pg log",
		SourceFile: "pkg/recent.go",
		SourceLine: 1,
		CreatedAt:  time.Now().UTC().Add(-5 * time.Minute),
		Attributes: map[string]string{},
	}); err != nil {
		t.Fatalf("InsertLogEntry recent: %v", err)
	}

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	n, err := s.DeleteLogEntriesBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteLogEntriesBefore: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 deleted, got %d", n)
	}

	// Recent should still exist
	list, err := s.ListLogEntries(ctx, domain.LogFilter{Limit: 500})
	if err != nil {
		t.Fatalf("ListLogEntries: %v", err)
	}
	var foundRecent bool
	for _, e := range list {
		if e.ID == idOld {
			t.Fatal("old entry should have been deleted")
		}
		if e.ID == idRecent {
			foundRecent = true
		}
	}
	if !foundRecent {
		t.Fatal("recent entry should not have been deleted")
	}
}

// ---------------------------------------------------------------------------
// Media
// ---------------------------------------------------------------------------

func TestPostgres_ListTeamMedia_DeleteMediaItem(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "ltm-pg-"+uuid.NewString(), "ltm@pg.test", "LTM PG")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ltm-pg-" + uuid.NewString()})

	// Empty
	items, err := s.ListTeamMedia(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListTeamMedia empty: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0, got %d", len(items))
	}

	item1, err := s.CreateMediaItem(ctx, domain.MediaItem{
		TeamID:    team.ID,
		Sha256:    "pg-sha-a-" + uuid.NewString(),
		Filename:  "a.png",
		MimeType:  "image/png",
		SizeBytes: 1024,
	})
	if err != nil {
		t.Fatalf("CreateMediaItem: %v", err)
	}

	item2, err := s.CreateMediaItem(ctx, domain.MediaItem{
		TeamID:    team.ID,
		Sha256:    "pg-sha-b-" + uuid.NewString(),
		Filename:  "b.png",
		MimeType:  "image/png",
		SizeBytes: 2048,
	})
	if err != nil {
		t.Fatalf("CreateMediaItem 2: %v", err)
	}
	_ = item2

	items, err = s.ListTeamMedia(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListTeamMedia: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2, got %d", len(items))
	}

	// Delete
	if err := s.DeleteMediaItem(ctx, team.ID, item1.ID); err != nil {
		t.Fatalf("DeleteMediaItem: %v", err)
	}

	items, err = s.ListTeamMedia(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListTeamMedia after delete: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 after delete, got %d", len(items))
	}
}

func TestPostgres_MediaProviderMapping_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "mpm-pg-"+uuid.NewString(), "mpm@pg.test", "MPM PG")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "mpm-pg-" + uuid.NewString()})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://mpm-pg.test", Username: "mpm", AccessToken: "at",
	})
	item, _ := s.CreateMediaItem(ctx, domain.MediaItem{
		TeamID:    team.ID,
		Sha256:    "pg-sha-mpm-" + uuid.NewString(),
		Filename:  "img.png",
		MimeType:  "image/png",
		SizeBytes: 512,
	})

	// Get non-existent
	_, err := s.GetMediaProviderMapping(ctx, item.ID, acc.ID)
	if err == nil {
		t.Fatal("expected error for missing mapping")
	}

	// Upsert
	if err := s.UpsertMediaProviderMapping(ctx, domain.MediaProviderMapping{
		MediaID:   item.ID,
		AccountID: acc.ID,
		RemoteID:  "pg-remote-123",
	}); err != nil {
		t.Fatalf("UpsertMediaProviderMapping: %v", err)
	}

	// Get
	m, err := s.GetMediaProviderMapping(ctx, item.ID, acc.ID)
	if err != nil {
		t.Fatalf("GetMediaProviderMapping: %v", err)
	}
	if m.RemoteID != "pg-remote-123" {
		t.Fatalf("RemoteID: got %q", m.RemoteID)
	}

	// Upsert again (update)
	exp := time.Now().UTC().Add(24 * time.Hour)
	if err := s.UpsertMediaProviderMapping(ctx, domain.MediaProviderMapping{
		MediaID:   item.ID,
		AccountID: acc.ID,
		RemoteID:  "pg-remote-456",
		ExpiresAt: &exp,
	}); err != nil {
		t.Fatalf("UpsertMediaProviderMapping update: %v", err)
	}

	m2, err := s.GetMediaProviderMapping(ctx, item.ID, acc.ID)
	if err != nil {
		t.Fatalf("GetMediaProviderMapping after update: %v", err)
	}
	if m2.RemoteID != "pg-remote-456" {
		t.Fatalf("RemoteID after update: got %q", m2.RemoteID)
	}
	if m2.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt set after update")
	}
}

// ---------------------------------------------------------------------------
// AccountMetrics
// ---------------------------------------------------------------------------

func TestPostgres_AccountMetrics_UpsertAndHistory(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "am-pg-"+uuid.NewString(), "am@pg.test", "AM PG")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "am-pg-" + uuid.NewString()})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://am-pg.test", Username: "am", AccessToken: "at",
	})

	recordedAt := time.Now().UTC()
	metrics := map[string]int64{
		"followers": 200,
		"following": 75,
		"posts":     300,
	}

	if err := s.UpsertAccountMetrics(ctx, acc.ID, metrics, recordedAt); err != nil {
		t.Fatalf("UpsertAccountMetrics: %v", err)
	}

	series, err := s.GetTeamAccountMetricHistorySeries(ctx, team.ID, "all", 30)
	if err != nil {
		t.Fatalf("GetTeamAccountMetricHistorySeries: %v", err)
	}
	if len(series) == 0 {
		t.Fatal("expected at least one history point")
	}

	today := recordedAt.Format("2006-01-02")
	var found bool
	for _, p := range series {
		if p.Date == today {
			found = true
			if p.Followers != 200 {
				t.Fatalf("Followers: got %d, want 200", p.Followers)
			}
		}
	}
	if !found {
		t.Fatalf("today's data point not found (date=%q)", today)
	}

	// Filter by account
	seriesFiltered, err := s.GetTeamAccountMetricHistorySeries(ctx, team.ID, acc.ID, 30)
	if err != nil {
		t.Fatalf("GetTeamAccountMetricHistorySeries by account: %v", err)
	}
	if len(seriesFiltered) == 0 {
		t.Fatal("expected points when filtered by account")
	}

	// Empty metrics is no-op
	if err := s.UpsertAccountMetrics(ctx, acc.ID, map[string]int64{}, recordedAt); err != nil {
		t.Fatalf("UpsertAccountMetrics empty: %v", err)
	}
}

func TestPostgres_ListAccountsForMetricsSync(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "lam-pg-"+uuid.NewString(), "lam@pg.test", "LAM PG")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "lam-pg-" + uuid.NewString()})

	acc1, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://lam1-pg.test", Username: "lam1", AccessToken: "at1",
	})
	acc2, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://lam2-pg.test", Username: "lam2", AccessToken: "at2",
	})

	list, err := s.ListAccountsForMetricsSync(ctx, 1000)
	if err != nil {
		t.Fatalf("ListAccountsForMetricsSync: %v", err)
	}

	var foundAcc1, foundAcc2 bool
	for _, a := range list {
		if a.ID == acc1.ID {
			foundAcc1 = true
		}
		if a.ID == acc2.ID {
			foundAcc2 = true
		}
	}
	if !foundAcc1 || !foundAcc2 {
		t.Fatalf("not all accounts found: acc1=%v acc2=%v", foundAcc1, foundAcc2)
	}
}

// ---------------------------------------------------------------------------
// ExternalPostMonitor
// ---------------------------------------------------------------------------

func TestPostgres_ExternalPostMonitor_ListAndSync(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "epm-pg-"+uuid.NewString(), "epm@pg.test", "EPM PG")
	team1, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "epm1-pg-" + uuid.NewString()})
	team2, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "epm2-pg-" + uuid.NewString()})

	// Enable team1
	_, err := s.UpsertExternalPostMonitorSettings(ctx, team1.ID, domain.UpsertExternalPostMonitorInput{Enabled: true})
	if err != nil {
		t.Fatalf("UpsertExternalPostMonitorSettings team1: %v", err)
	}

	// Enable then disable team2
	_, err = s.UpsertExternalPostMonitorSettings(ctx, team2.ID, domain.UpsertExternalPostMonitorInput{Enabled: true})
	if err != nil {
		t.Fatalf("UpsertExternalPostMonitorSettings team2 enable: %v", err)
	}
	_, err = s.UpsertExternalPostMonitorSettings(ctx, team2.ID, domain.UpsertExternalPostMonitorInput{Enabled: false})
	if err != nil {
		t.Fatalf("UpsertExternalPostMonitorSettings team2 disable: %v", err)
	}

	list, err := s.ListTeamsWithExternalPostMonitorEnabled(ctx, 200)
	if err != nil {
		t.Fatalf("ListTeamsWithExternalPostMonitorEnabled: %v", err)
	}
	var foundTeam1 bool
	for _, m := range list {
		if m.TeamID == team1.ID {
			foundTeam1 = true
		}
		if m.TeamID == team2.ID {
			t.Fatal("disabled team should not appear in enabled list")
		}
	}
	if !foundTeam1 {
		t.Fatal("team1 not found in enabled list")
	}

	// UpdateExternalPostMonitorSyncState
	syncAt := time.Now().UTC()
	if err := s.UpdateExternalPostMonitorSyncState(ctx, team1.ID, syncAt, false); err != nil {
		t.Fatalf("UpdateExternalPostMonitorSyncState: %v", err)
	}

	got, err := s.GetExternalPostMonitorSettings(ctx, team1.ID)
	if err != nil {
		t.Fatalf("GetExternalPostMonitorSettings: %v", err)
	}
	if got.LastSyncAt == nil {
		t.Fatal("expected LastSyncAt to be set")
	}
	if got.BackfillCompletedAt != nil {
		t.Fatal("expected BackfillCompletedAt=nil")
	}

	// With backfill
	if err := s.UpdateExternalPostMonitorSyncState(ctx, team1.ID, syncAt.Add(time.Minute), true); err != nil {
		t.Fatalf("UpdateExternalPostMonitorSyncState backfill: %v", err)
	}
	got2, err := s.GetExternalPostMonitorSettings(ctx, team1.ID)
	if err != nil {
		t.Fatalf("GetExternalPostMonitorSettings after backfill: %v", err)
	}
	if got2.BackfillCompletedAt == nil {
		t.Fatal("expected BackfillCompletedAt set after backfill")
	}
}

func TestPostgres_TargetExistsByRemotePostID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "ter-pg-"+uuid.NewString(), "ter@pg.test", "TER PG")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ter-pg-" + uuid.NewString()})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://ter-pg.test", Username: "ter", AccessToken: "at",
	})

	// Not exists
	exists, err := s.TargetExistsByRemotePostID(ctx, acc.ID, "pg-remote-123")
	if err != nil {
		t.Fatalf("TargetExistsByRemotePostID: %v", err)
	}
	if exists {
		t.Fatal("expected not exists before import")
	}

	// Create imported post
	if _, err := s.CreateImportedPost(ctx, team.ID, u.ID, domain.ImportedPostInput{
		AccountID:    acc.ID,
		RemotePostID: "pg-remote-123",
		Content:      "pg imported",
		PublishedAt:  time.Now().UTC().Add(-time.Hour),
		PublishedURL: "https://pg.example.com/posts/123",
	}); err != nil {
		t.Fatalf("CreateImportedPost: %v", err)
	}

	exists, err = s.TargetExistsByRemotePostID(ctx, acc.ID, "pg-remote-123")
	if err != nil {
		t.Fatalf("TargetExistsByRemotePostID after import: %v", err)
	}
	if !exists {
		t.Fatal("expected exists after import")
	}
}

// ---------------------------------------------------------------------------
// GetAccountsByIDsGlobal
// ---------------------------------------------------------------------------

func TestPostgres_GetAccountsByIDsGlobal(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "gag-pg-"+uuid.NewString(), "gag@pg.test", "GAG PG")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "gag-pg-" + uuid.NewString()})

	acc1, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://gag1-pg.test", Username: "gag1", AccessToken: "at1",
	})
	acc2, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://gag2-pg.test", Username: "gag2", AccessToken: "at2",
	})

	// Get both (same team)
	accounts, err := s.GetAccountsByIDsGlobal(ctx, []string{acc1.ID, acc2.ID})
	if err != nil {
		t.Fatalf("GetAccountsByIDsGlobal: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2, got %d", len(accounts))
	}

	// Empty IDs returns error
	if _, err := s.GetAccountsByIDsGlobal(ctx, nil); err == nil {
		t.Fatal("expected error for nil ids")
	}
	if _, err := s.GetAccountsByIDsGlobal(ctx, []string{}); err == nil {
		t.Fatal("expected error for empty ids")
	}

	// Non-existent ID should fail (count mismatch)
	if _, err := s.GetAccountsByIDsGlobal(ctx, []string{acc1.ID, uuid.NewString()}); err == nil {
		t.Fatal("expected error when account not found")
	}

	// Cross-team accounts should fail
	u2, _ := s.UpsertOIDCUser(ctx, "gag2-pg-"+uuid.NewString(), "gag2@pg.test", "GAG2 PG")
	team2, _ := s.CreateTeam(ctx, u2.ID, domain.CreateTeamInput{Name: "gag2-pg-" + uuid.NewString()})
	acc3, _ := s.CreateAccount(ctx, team2.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://gag3-pg.test", Username: "gag3", AccessToken: "at3",
	})
	if _, err := s.GetAccountsByIDsGlobal(ctx, []string{acc1.ID, acc3.ID}); err == nil {
		t.Fatal("expected error for accounts from different teams")
	}
}
