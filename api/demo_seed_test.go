package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
)

const demoSeedAdminToken = "demo-seed-admin-token"

func seedDemoAdmin(t *testing.T, s *sqlitestore.Store) string {
	t.Helper()
	if err := s.EnsureBootstrapAdmin(context.Background(), "admin@localhost", "Local Administrator", demoSeedAdminToken); err != nil {
		t.Fatalf("EnsureBootstrapAdmin: %v", err)
	}
	return demoSeedAdminToken
}

func postDemoSeed(t *testing.T, handler http.Handler, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/e2e/demo-seed", nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAdminDemoSeed_CreatesRichDemoTeam(t *testing.T) {
	ctx := context.Background()
	s := newValidateE2EStore(t)
	bearer := seedDemoAdmin(t, s)
	handler := validateE2EHandler(t, s)

	rec := postDemoSeed(t, handler, bearer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		TeamID  string `json:"team_id"`
		Created bool   `json:"created"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.TeamID == "" || !payload.Created {
		t.Fatalf("payload = %+v, want created team", payload)
	}

	accounts, err := s.ListTeamAccounts(ctx, payload.TeamID)
	if err != nil {
		t.Fatalf("ListTeamAccounts: %v", err)
	}
	if len(accounts) != 3 {
		t.Fatalf("accounts = %d, want 3", len(accounts))
	}

	posts, err := s.ListTeamPosts(ctx, payload.TeamID)
	if err != nil {
		t.Fatalf("ListTeamPosts: %v", err)
	}
	var posted, pending, drafts int
	for _, p := range posts {
		switch p.Status {
		case domain.PostStatusPosted:
			posted++
		case domain.PostStatusPending:
			pending++
		case domain.PostStatusDraft:
			drafts++
		}
	}
	if posted < 4 || pending < 3 || drafts < 1 {
		t.Fatalf("post mix posted=%d pending=%d drafts=%d, want >=4/>=3/>=1", posted, pending, drafts)
	}

	// Published demo posts must carry engagement so analytics is non-zero.
	summary, err := s.GetTeamAnalytics(ctx, payload.TeamID, 5)
	if err != nil {
		t.Fatalf("GetTeamAnalytics: %v", err)
	}
	var engagement int64
	for _, v := range summary.MetricsTotal {
		engagement += v
	}
	if engagement == 0 {
		t.Fatal("total engagement = 0, want > 0")
	}

	// Engagement history must span multiple days for the trend chart.
	series, err := s.GetTeamMetricHistorySeries(ctx, payload.TeamID, "likes", 30)
	if err != nil {
		t.Fatalf("GetTeamMetricHistorySeries: %v", err)
	}
	if len(series) < 10 {
		t.Fatalf("likes series = %d points, want >= 10", len(series))
	}

	// Review queue and automations give those screens content.
	reviews, err := s.ListAutomationReviewDrafts(ctx, payload.TeamID, 10)
	if err != nil {
		t.Fatalf("ListAutomationReviewDrafts: %v", err)
	}
	if len(reviews) == 0 {
		t.Fatal("review queue empty, want seeded drafts")
	}
	feeds, err := s.ListRSSFeedConfigs(ctx, payload.TeamID)
	if err != nil {
		t.Fatalf("ListRSSFeedConfigs: %v", err)
	}
	if len(feeds) == 0 {
		t.Fatal("rss feeds empty, want seeded feed")
	}
	templates, err := s.ListPostTemplates(ctx, payload.TeamID)
	if err != nil {
		t.Fatalf("ListPostTemplates: %v", err)
	}
	if len(templates) == 0 {
		t.Fatal("post templates empty, want seeded recurring template")
	}
}

func TestAdminDemoSeed_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := newValidateE2EStore(t)
	bearer := seedDemoAdmin(t, s)
	handler := validateE2EHandler(t, s)

	first := postDemoSeed(t, handler, bearer)
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d body=%s", first.Code, first.Body.String())
	}
	var firstPayload struct {
		TeamID string `json:"team_id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstPayload); err != nil {
		t.Fatalf("decode first: %v", err)
	}

	second := postDemoSeed(t, handler, bearer)
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d body=%s", second.Code, second.Body.String())
	}
	var secondPayload struct {
		TeamID  string `json:"team_id"`
		Created bool   `json:"created"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondPayload); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if secondPayload.Created {
		t.Fatal("second call reports created=true, want reuse")
	}
	if secondPayload.TeamID != firstPayload.TeamID {
		t.Fatalf("team id changed: %s -> %s", firstPayload.TeamID, secondPayload.TeamID)
	}

	posts, err := s.ListTeamPosts(ctx, firstPayload.TeamID)
	if err != nil {
		t.Fatalf("ListTeamPosts: %v", err)
	}
	firstCount := len(posts)
	if firstCount == 0 {
		t.Fatal("no posts after seeding")
	}
}

func TestAdminDemoSeed_RequiresAdmin(t *testing.T) {
	s := newValidateE2EStore(t)
	// The very first OIDC user becomes admin; burn that slot so the
	// seedValidateE2E user below is a regular member.
	if _, err := s.UpsertOIDCUser(context.Background(), "first-admin", "first@val.test", "First Admin"); err != nil {
		t.Fatalf("UpsertOIDCUser: %v", err)
	}
	bearer, _, _, _ := seedValidateE2E(t, s)
	handler := validateE2EHandler(t, s)

	rec := postDemoSeed(t, handler, bearer)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
