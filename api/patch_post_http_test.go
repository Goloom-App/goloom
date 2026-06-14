package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func TestHandleUpdatePost_PartialScheduledAt_Persists(t *testing.T) {
	s := newMemorySQLite(t)
	token, teamID, accID := seedTeamWithEditorPost(t, s)
	h := analyticsTestHandler(t, s)

	ctx := context.Background()
	when := time.Date(2026, 6, 10, 14, 30, 0, 0, time.UTC)
	u, _ := s.UpsertOIDCUser(ctx, "patch-http", "ph@x", "PH")
	principal := domain.AuthenticatedPrincipal{User: u}
	post, err := s.CreateScheduledPost(ctx, teamID, principal, domain.CreatePostInput{
		Content: "body", ScheduledAt: when, TargetAccounts: []string{accID},
	})
	if err != nil {
		t.Fatal(err)
	}

	newWhen := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	body, _ := json.Marshal(map[string]string{
		"scheduled_at": newWhen.Format(time.RFC3339),
	})
	req := httptest.NewRequest(http.MethodPatch, "/v1/teams/"+teamID+"/posts/"+post.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("teamID", teamID)
	req.SetPathValue("postID", post.ID)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	got, err := s.GetScheduledPost(ctx, teamID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ScheduledAt.Equal(newWhen) {
		t.Fatalf("scheduled_at=%v want %v", got.ScheduledAt, newWhen)
	}
}

func seedTeamWithEditorPost(t *testing.T, s *sqlitestore.Store) (bearer, teamID, accID string) {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "patch-ed-"+uuid.NewString(), "pe@x", "PE")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "pe-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	plain, _, err := s.CreateUserAPIToken(ctx, u.ID, "patch", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "x", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	return plain, team.ID, acc.ID
}
