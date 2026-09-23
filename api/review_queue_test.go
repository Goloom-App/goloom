package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestReviewQueueListsAutomationDrafts(t *testing.T) {
	ctx := context.Background()
	s := newValidateE2EStore(t)
	bearer, teamID, bsID, _ := seedValidateE2E(t, s)
	handler := validateE2EHandler(t, s)
	users, err := s.ListUsers(ctx)
	if err != nil || len(users) == 0 {
		t.Fatalf("ListUsers: %v", err)
	}
	authorID := users[0].ID

	scheduled := time.Now().UTC().Add(-3 * time.Hour)
	_, err = s.CreateScheduledPost(ctx, teamID, domain.AuthenticatedPrincipal{
		User: domain.User{ID: authorID},
		Kind: "api_token",
	}, domain.CreatePostInput{
		Title:          "RSS Review Item",
		Content:        "Draft from automation",
		ScheduledAt:    scheduled,
		TargetAccounts: []string{bsID},
		Draft:          true,
		Source:         domain.PostSourceAutomation,
	})
	if err != nil {
		t.Fatalf("CreateScheduledPost: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/review-queue", nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []domain.ReviewQueueItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(payload.Items))
	}
	if !payload.Items[0].IsOverdue {
		t.Fatal("expected overdue draft")
	}
	if payload.Items[0].Content != "Draft from automation" {
		t.Fatalf("content = %q", payload.Items[0].Content)
	}
}

func TestAdminSeedE2EAccountCreatesUsableAccount(t *testing.T) {
	ctx := context.Background()
	s := newValidateE2EStore(t)
	bearer, teamID, _, _ := seedValidateE2E(t, s)
	h := validateE2EHandler(t, s)

	body, _ := json.Marshal(map[string]any{"team_id": teamID})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/e2e/account", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	accounts, err := s.ListTeamAccounts(ctx, teamID)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, acc := range accounts {
		if strings.HasPrefix(acc.Username, "e2e-") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("seeded account missing from team accounts: %#v", accounts)
	}

	// The seeded account must be usable as a real post target.
	users, err := s.ListUsers(ctx)
	if err != nil || len(users) == 0 {
		t.Fatalf("ListUsers: %v", err)
	}
	post, err := s.CreateScheduledPost(ctx, teamID, domain.AuthenticatedPrincipal{
		User: domain.User{ID: users[0].ID},
		Kind: "api_token",
	}, domain.CreatePostInput{
		Title:          "Targets seeded account",
		Content:        "x",
		ScheduledAt:    time.Now().UTC().Add(24 * time.Hour),
		TargetAccounts: []string{accounts[0].ID},
	})
	if err != nil {
		t.Fatalf("CreateScheduledPost with seeded account: %v", err)
	}
	if post.ID == "" {
		t.Fatal("no post id")
	}
}

