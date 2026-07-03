package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// logs.go: ArchiveLogEntry, UnarchiveLogEntry, DeleteLogEntry, DeleteLogEntriesBefore
// ---------------------------------------------------------------------------

func makeLogEntry(t *testing.T, s interface {
	InsertLogEntry(ctx context.Context, e domain.LogEntry) error
}, msg, level string, at time.Time) string {
	t.Helper()
	id := uuid.NewString()
	if err := s.InsertLogEntry(context.Background(), domain.LogEntry{
		ID:         id,
		Level:      level,
		Message:    msg,
		SourceFile: "pkg/foo/bar.go",
		SourceLine: 42,
		CreatedAt:  at,
		Attributes: map[string]string{"key": "val"},
	}); err != nil {
		t.Fatalf("InsertLogEntry: %v", err)
	}
	return id
}

func TestSQLite_ArchiveAndUnarchiveLogEntry(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	id := makeLogEntry(t, s, "test message", "info", time.Now().UTC())

	// Initially not archived
	list, err := s.ListLogEntries(ctx, domain.LogFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListLogEntries: %v", err)
	}
	var found bool
	for _, e := range list {
		if e.ID == id {
			found = true
			if e.ArchivedAt != nil {
				t.Fatal("expected ArchivedAt=nil initially")
			}
		}
	}
	if !found {
		t.Fatal("entry not found in listing")
	}

	// Archive
	if err := s.ArchiveLogEntry(ctx, id); err != nil {
		t.Fatalf("ArchiveLogEntry: %v", err)
	}

	// Should not appear in non-archived filter
	notArchived := false
	archivedFilter, err := s.ListLogEntries(ctx, domain.LogFilter{Limit: 50, Archived: boolPtr(false)})
	if err != nil {
		t.Fatalf("ListLogEntries archived=false: %v", err)
	}
	for _, e := range archivedFilter {
		if e.ID == id {
			notArchived = true
		}
	}
	if notArchived {
		t.Fatal("archived entry should not appear in archived=false filter")
	}

	// Should appear in archived filter
	archivedList, err := s.ListLogEntries(ctx, domain.LogFilter{Limit: 50, Archived: boolPtr(true)})
	if err != nil {
		t.Fatalf("ListLogEntries archived=true: %v", err)
	}
	var foundArchived bool
	for _, e := range archivedList {
		if e.ID == id {
			foundArchived = true
			if e.ArchivedAt == nil {
				t.Fatal("expected ArchivedAt set after archive")
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

	// Should appear in non-archived filter again
	afterUnarchive, err := s.ListLogEntries(ctx, domain.LogFilter{Limit: 50, Archived: boolPtr(false)})
	if err != nil {
		t.Fatalf("ListLogEntries after unarchive: %v", err)
	}
	var foundUnarchived bool
	for _, e := range afterUnarchive {
		if e.ID == id && e.ArchivedAt == nil {
			foundUnarchived = true
		}
	}
	if !foundUnarchived {
		t.Fatal("entry not found after unarchive")
	}
}

func TestSQLite_DeleteLogEntry(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	id := makeLogEntry(t, s, "to delete", "warn", time.Now().UTC())

	if err := s.DeleteLogEntry(ctx, id); err != nil {
		t.Fatalf("DeleteLogEntry: %v", err)
	}

	list, err := s.ListLogEntries(ctx, domain.LogFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListLogEntries after delete: %v", err)
	}
	for _, e := range list {
		if e.ID == id {
			t.Fatal("deleted entry still found")
		}
	}
}

func TestSQLite_DeleteLogEntriesBefore(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	old := time.Now().UTC().Add(-48 * time.Hour)
	recent := time.Now().UTC().Add(-5 * time.Minute)

	idOld := makeLogEntry(t, s, "old message", "info", old)
	idRecent := makeLogEntry(t, s, "recent message", "info", recent)

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	n, err := s.DeleteLogEntriesBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteLogEntriesBefore: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 deleted, got %d", n)
	}

	list, err := s.ListLogEntries(ctx, domain.LogFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListLogEntries: %v", err)
	}
	for _, e := range list {
		if e.ID == idOld {
			t.Fatal("old entry should have been deleted")
		}
	}
	var foundRecent bool
	for _, e := range list {
		if e.ID == idRecent {
			foundRecent = true
		}
	}
	if !foundRecent {
		t.Fatal("recent entry should not have been deleted")
	}
}

func boolPtr(b bool) *bool { return &b }

// ---------------------------------------------------------------------------
// media.go: ListTeamMedia, DeleteMediaItem, GetMediaProviderMapping, UpsertMediaProviderMapping
// ---------------------------------------------------------------------------

func TestSQLite_ListTeamMedia(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "ltm-"+uuid.NewString(), "ltm@test.com", "LTM")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ltm-" + uuid.NewString()})

	// Empty
	items, err := s.ListTeamMedia(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListTeamMedia empty: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0, got %d", len(items))
	}

	// Add items
	w, h := 100, 100
	item1, err := s.CreateMediaItem(ctx, domain.MediaItem{
		TeamID:    team.ID,
		Sha256:    "sha-a-" + uuid.NewString(),
		Filename:  "a.png",
		MimeType:  "image/png",
		SizeBytes: 1024,
		Width:     &w,
		Height:    &h,
	})
	if err != nil {
		t.Fatalf("CreateMediaItem 1: %v", err)
	}

	item2, err := s.CreateMediaItem(ctx, domain.MediaItem{
		TeamID:    team.ID,
		Sha256:    "sha-b-" + uuid.NewString(),
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
	// Ordered by created_at desc, so item2 first
	if items[0].ID != item2.ID {
		t.Fatalf("expected item2 first (newest), got %q", items[0].ID)
	}

	// Isolation: another team sees 0
	u2, _ := s.UpsertOIDCUser(ctx, "ltm2-"+uuid.NewString(), "ltm2@test.com", "LTM2")
	team2, _ := s.CreateTeam(ctx, u2.ID, domain.CreateTeamInput{Name: "ltm2-" + uuid.NewString()})
	items2, _ := s.ListTeamMedia(ctx, team2.ID)
	if len(items2) != 0 {
		t.Fatalf("team2 should see 0 media, got %d", len(items2))
	}

	// DeleteMediaItem
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
	if items[0].ID != item2.ID {
		t.Fatalf("wrong item remaining: %q", items[0].ID)
	}
}

func TestSQLite_MediaProviderMapping_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "mpm-"+uuid.NewString(), "mpm@test.com", "MPM")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "mpm-" + uuid.NewString()})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://mpm.test", Username: "mpm", AccessToken: "at",
	})
	item, _ := s.CreateMediaItem(ctx, domain.MediaItem{
		TeamID:    team.ID,
		Sha256:    "sha-mpm-" + uuid.NewString(),
		Filename:  "img.png",
		MimeType:  "image/png",
		SizeBytes: 512,
	})

	// Get non-existent - returns ErrNoRows
	_, err := s.GetMediaProviderMapping(ctx, item.ID, acc.ID)
	if err == nil {
		t.Fatal("expected error for missing mapping")
	}

	// Upsert
	if err := s.UpsertMediaProviderMapping(ctx, domain.MediaProviderMapping{
		MediaID:   item.ID,
		AccountID: acc.ID,
		RemoteID:  "remote-123",
	}); err != nil {
		t.Fatalf("UpsertMediaProviderMapping: %v", err)
	}

	// Get
	m, err := s.GetMediaProviderMapping(ctx, item.ID, acc.ID)
	if err != nil {
		t.Fatalf("GetMediaProviderMapping: %v", err)
	}
	if m.MediaID != item.ID {
		t.Fatalf("MediaID: got %q", m.MediaID)
	}
	if m.AccountID != acc.ID {
		t.Fatalf("AccountID: got %q", m.AccountID)
	}
	if m.RemoteID != "remote-123" {
		t.Fatalf("RemoteID: got %q", m.RemoteID)
	}
	if m.ExpiresAt != nil {
		t.Fatal("expected ExpiresAt=nil")
	}

	// Upsert again (update) with expiry
	exp := time.Now().UTC().Add(24 * time.Hour)
	if err := s.UpsertMediaProviderMapping(ctx, domain.MediaProviderMapping{
		MediaID:   item.ID,
		AccountID: acc.ID,
		RemoteID:  "remote-456",
		ExpiresAt: &exp,
	}); err != nil {
		t.Fatalf("UpsertMediaProviderMapping update: %v", err)
	}

	m2, err := s.GetMediaProviderMapping(ctx, item.ID, acc.ID)
	if err != nil {
		t.Fatalf("GetMediaProviderMapping after update: %v", err)
	}
	if m2.RemoteID != "remote-456" {
		t.Fatalf("RemoteID after update: got %q", m2.RemoteID)
	}
	if m2.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt set after update")
	}
}

// ---------------------------------------------------------------------------
// account_metrics.go: ListAccountsForMetricsSync, UpsertAccountMetrics, GetTeamAccountMetricHistorySeries
// ---------------------------------------------------------------------------

func TestSQLite_AccountMetrics_UpsertAndHistory(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "am-"+uuid.NewString(), "am@test.com", "AM")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "am-" + uuid.NewString()})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://am.test", Username: "am", AccessToken: "at",
	})

	recordedAt := time.Now().UTC()
	metrics := map[string]int64{
		"followers": 100,
		"following": 50,
		"posts":     200,
	}

	// Upsert metrics
	if err := s.UpsertAccountMetrics(ctx, acc.ID, metrics, recordedAt); err != nil {
		t.Fatalf("UpsertAccountMetrics: %v", err)
	}

	// GetTeamAccountMetricHistorySeries for all accounts
	series, err := s.GetTeamAccountMetricHistorySeries(ctx, team.ID, "all", 30)
	if err != nil {
		t.Fatalf("GetTeamAccountMetricHistorySeries: %v", err)
	}
	if len(series) == 0 {
		t.Fatal("expected at least one history point")
	}

	// Find today's entry
	today := recordedAt.Format("2006-01-02")
	var found bool
	for _, p := range series {
		if p.Date == today {
			found = true
			if p.Followers != 100 {
				t.Fatalf("Followers: got %d, want 100", p.Followers)
			}
			if p.Following != 50 {
				t.Fatalf("Following: got %d, want 50", p.Following)
			}
			if p.Posts != 200 {
				t.Fatalf("Posts: got %d, want 200", p.Posts)
			}
		}
	}
	if !found {
		t.Fatalf("today's data point not found in series (date=%q, series=%v)", today, series)
	}

	// Filter by specific account ID
	seriesFiltered, err := s.GetTeamAccountMetricHistorySeries(ctx, team.ID, acc.ID, 30)
	if err != nil {
		t.Fatalf("GetTeamAccountMetricHistorySeries by accountID: %v", err)
	}
	if len(seriesFiltered) == 0 {
		t.Fatal("expected at least one history point when filtered by account")
	}

	// Upsert again with updated values (on conflict update)
	metrics2 := map[string]int64{"followers": 110}
	if err := s.UpsertAccountMetrics(ctx, acc.ID, metrics2, recordedAt); err != nil {
		t.Fatalf("UpsertAccountMetrics update: %v", err)
	}

	// Empty metrics is a no-op
	if err := s.UpsertAccountMetrics(ctx, acc.ID, map[string]int64{}, recordedAt); err != nil {
		t.Fatalf("UpsertAccountMetrics empty: %v", err)
	}
}

func TestSQLite_ListAccountsForMetricsSync(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "lam-"+uuid.NewString(), "lam@test.com", "LAM")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "lam-" + uuid.NewString()})

	// Create accounts
	acc1, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://lam1.test", Username: "lam1", AccessToken: "at1",
	})
	acc2, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://lam2.test", Username: "lam2", AccessToken: "at2",
	})

	list, err := s.ListAccountsForMetricsSync(ctx, 100)
	if err != nil {
		t.Fatalf("ListAccountsForMetricsSync: %v", err)
	}

	// Should include our accounts
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

	// Limit 1
	list1, err := s.ListAccountsForMetricsSync(ctx, 1)
	if err != nil {
		t.Fatalf("ListAccountsForMetricsSync limit=1: %v", err)
	}
	if len(list1) != 1 {
		t.Fatalf("expected 1 with limit=1, got %d", len(list1))
	}
}

// ---------------------------------------------------------------------------
// external_post_monitor.go: ListTeamsWithExternalPostMonitorEnabled, UpdateExternalPostMonitorSyncState, TargetExistsByRemotePostID
// ---------------------------------------------------------------------------

func TestSQLite_ExternalPostMonitor_ListAndSync(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "epm-"+uuid.NewString(), "epm@test.com", "EPM")
	team1, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "epm1-" + uuid.NewString()})
	team2, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "epm2-" + uuid.NewString()})

	// Neither team has external post monitor - list should be empty
	list, err := s.ListTeamsWithExternalPostMonitorEnabled(ctx, 100)
	if err != nil {
		t.Fatalf("ListTeamsWithExternalPostMonitorEnabled empty: %v", err)
	}
	for _, m := range list {
		if m.TeamID == team1.ID || m.TeamID == team2.ID {
			t.Fatal("unexpected team in enabled list before enabling")
		}
	}

	// Enable for team1
	_, err = s.UpsertExternalPostMonitorSettings(ctx, team1.ID, domain.UpsertExternalPostMonitorInput{Enabled: true})
	if err != nil {
		t.Fatalf("UpsertExternalPostMonitorSettings enable team1: %v", err)
	}

	// Enable for team2 then disable
	_, err = s.UpsertExternalPostMonitorSettings(ctx, team2.ID, domain.UpsertExternalPostMonitorInput{Enabled: true})
	if err != nil {
		t.Fatalf("UpsertExternalPostMonitorSettings enable team2: %v", err)
	}
	_, err = s.UpsertExternalPostMonitorSettings(ctx, team2.ID, domain.UpsertExternalPostMonitorInput{Enabled: false})
	if err != nil {
		t.Fatalf("UpsertExternalPostMonitorSettings disable team2: %v", err)
	}

	// List should now contain only team1
	list, err = s.ListTeamsWithExternalPostMonitorEnabled(ctx, 100)
	if err != nil {
		t.Fatalf("ListTeamsWithExternalPostMonitorEnabled: %v", err)
	}
	var foundTeam1 bool
	for _, m := range list {
		if m.TeamID == team1.ID {
			foundTeam1 = true
		}
		if m.TeamID == team2.ID {
			t.Fatal("disabled team2 should not be in enabled list")
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
		t.Fatal("expected LastSyncAt to be set after sync")
	}
	if got.BackfillCompletedAt != nil {
		t.Fatal("expected BackfillCompletedAt=nil when backfillCompleted=false")
	}

	// Sync with backfill completed
	if err := s.UpdateExternalPostMonitorSyncState(ctx, team1.ID, syncAt.Add(time.Minute), true); err != nil {
		t.Fatalf("UpdateExternalPostMonitorSyncState backfill: %v", err)
	}
	got2, err := s.GetExternalPostMonitorSettings(ctx, team1.ID)
	if err != nil {
		t.Fatalf("GetExternalPostMonitorSettings after backfill: %v", err)
	}
	if got2.BackfillCompletedAt == nil {
		t.Fatal("expected BackfillCompletedAt to be set after backfill complete")
	}

	// UpdateExternalPostMonitorSyncState for non-existent team is a no-op (no error)
	if err := s.UpdateExternalPostMonitorSyncState(ctx, uuid.NewString(), syncAt, false); err != nil {
		t.Fatalf("UpdateExternalPostMonitorSyncState non-existent: %v", err)
	}
}

func TestSQLite_TargetExistsByRemotePostID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "ter-"+uuid.NewString(), "ter@test.com", "TER")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ter-" + uuid.NewString()})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://ter.test", Username: "ter", AccessToken: "at",
	})

	// Not exists initially
	exists, err := s.TargetExistsByRemotePostID(ctx, acc.ID, "remote-123")
	if err != nil {
		t.Fatalf("TargetExistsByRemotePostID: %v", err)
	}
	if exists {
		t.Fatal("expected not exists before import")
	}

	// Create an imported post with remote_post_id
	principal := domain.AuthenticatedPrincipal{User: u}
	_, err = s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content:        "imported content",
		ScheduledAt:    time.Now().UTC().Add(-time.Hour),
		TargetAccounts: []string{acc.ID},
	})
	if err != nil {
		t.Fatalf("CreateScheduledPost: %v", err)
	}

	// Import directly
	_, err = s.CreateImportedPost(ctx, team.ID, u.ID, domain.ImportedPostInput{
		AccountID:    acc.ID,
		RemotePostID: "remote-123",
		Content:      "imported",
		PublishedAt:  time.Now().UTC().Add(-time.Hour),
		PublishedURL: "https://example.com/posts/123",
	})
	if err != nil {
		t.Fatalf("CreateImportedPost: %v", err)
	}

	// Mark the imported post's target as posted with the remote post ID
	// We need to mark it through the store
	// Actually TargetExistsByRemotePostID relies on scheduled_post_targets with status=posted and remote_post_id set
	// The imported post created via CreateImportedPost already sets status=posted
	exists, err = s.TargetExistsByRemotePostID(ctx, acc.ID, "remote-123")
	if err != nil {
		t.Fatalf("TargetExistsByRemotePostID after import: %v", err)
	}
	if !exists {
		t.Fatal("expected exists after import with remote_post_id")
	}

	// Different remote ID - not exists
	exists2, err := s.TargetExistsByRemotePostID(ctx, acc.ID, "remote-999")
	if err != nil {
		t.Fatalf("TargetExistsByRemotePostID different id: %v", err)
	}
	if exists2 {
		t.Fatal("expected not exists for different remote post ID")
	}
}

// ---------------------------------------------------------------------------
// personal.go: GetAccountsByIDsGlobal
// ---------------------------------------------------------------------------

func TestSQLite_GetAccountsByIDsGlobal(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "gag-"+uuid.NewString(), "gag@test.com", "GAG")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "gag-" + uuid.NewString()})

	acc1, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://gag1.test", Username: "gag1", AccessToken: "at1",
	})
	acc2, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://gag2.test", Username: "gag2", AccessToken: "at2",
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
	_, err = s.GetAccountsByIDsGlobal(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil ids")
	}
	_, err = s.GetAccountsByIDsGlobal(ctx, []string{})
	if err == nil {
		t.Fatal("expected error for empty ids")
	}

	// One non-existent ID should fail (count mismatch)
	_, err = s.GetAccountsByIDsGlobal(ctx, []string{acc1.ID, uuid.NewString()})
	if err == nil {
		t.Fatal("expected error when account not found")
	}

	// Accounts from different teams should fail
	u2, _ := s.UpsertOIDCUser(ctx, "gag2-"+uuid.NewString(), "gag2@test.com", "GAG2")
	team2, _ := s.CreateTeam(ctx, u2.ID, domain.CreateTeamInput{Name: "gag2-" + uuid.NewString()})
	acc3, _ := s.CreateAccount(ctx, team2.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://gag3.test", Username: "gag3", AccessToken: "at3",
	})
	_, err = s.GetAccountsByIDsGlobal(ctx, []string{acc1.ID, acc3.ID})
	if err == nil {
		t.Fatal("expected error when accounts from different teams")
	}
}
