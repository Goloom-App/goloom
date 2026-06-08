package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

