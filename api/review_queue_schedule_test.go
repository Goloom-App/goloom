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
)

// seedAutomationReviewDraftForSchedule comment placeholder removed.

func TestReviewQueuePastTimeScheduleRejected(t *testing.T) {
	ctx := context.Background()
	s := newValidateE2EStore(t)
	bearer, teamID, accID, _ := seedValidateE2E(t, s)
	h := validateE2EHandler(t, s)
	authorID := mustFirstUserID(t, s)

	draft, err := s.CreateScheduledPost(ctx, teamID, domain.AuthenticatedPrincipal{
		User: domain.User{ID: authorID},
		Kind: "api_token",
	}, domain.CreatePostInput{
		Title:          "RSS Review Item",
		Content:        "Draft from automation",
		ScheduledAt:    time.Now().UTC().Add(-3 * time.Hour),
		TargetAccounts: []string{accID},
		Draft:          true,
		Source:         domain.PostSourceAutomation,
	})
	if err != nil {
		t.Fatalf("CreateScheduledPost: %v", err)
	}

	// Queue "Schedule" with a past timestamp must be rejected, and the draft
	// must not be moved into the scheduler feed (no immediate publish).
	body, _ := json.Marshal(map[string]any{
		"scheduled_at":    time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
		"draft":           false,
		"review_complete": true,
	})
	req := httptest.NewRequest(http.MethodPatch, "/v1/teams/"+teamID+"/posts/"+draft.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Error-Code"); got != "past_scheduled_at" {
		t.Fatalf("X-Error-Code = %q, want past_scheduled_at", got)
	}

	got, err := s.GetScheduledPost(ctx, teamID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.PostStatusDraft {
		t.Fatalf("status = %s, want draft (must not publish)", got.Status)
	}
}

func TestReviewQueuePublishNowKeepsEditedContent(t *testing.T) {
	ctx := context.Background()
	s := newValidateE2EStore(t)
	bearer, teamID, accID, _ := seedValidateE2E(t, s)
	h := validateE2EHandler(t, s)
	authorID := mustFirstUserID(t, s)

	draft, err := s.CreateScheduledPost(ctx, teamID, domain.AuthenticatedPrincipal{
		User: domain.User{ID: authorID},
		Kind: "api_token",
	}, domain.CreatePostInput{
		Title:          "Edited automation title",
		Content:        "Edited automation body that must survive the queue action",
		ScheduledAt:    time.Now().UTC().Add(-2 * time.Hour),
		TargetAccounts: []string{accID},
		Draft:          true,
		Source:         domain.PostSourceAutomation,
	})
	if err != nil {
		t.Fatalf("CreateScheduledPost: %v", err)
	}

	// Queue "Publish now" sends only the hint + review-transition signal — no
	// cached title/content/targets.
	body, _ := json.Marshal(map[string]any{"publish_now": true, "review_complete": true})
	req := httptest.NewRequest(http.MethodPatch, "/v1/teams/"+teamID+"/posts/"+draft.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	got, err := s.GetScheduledPost(ctx, teamID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.PostStatusPending {
		t.Fatalf("status = %s, want pending (scheduler takes it on next poll)", got.Status)
	}
	if got.Title != "Edited automation title" {
		t.Fatalf("title = %q, want edited title preserved", got.Title)
	}
	if got.Content != "Edited automation body that must survive the queue action" {
		t.Fatalf("content = %q, want edited content preserved", got.Content)
	}
	if !got.ScheduledAt.After(time.Now().Add(-30 * time.Second)) {
		t.Fatalf("scheduled_at = %v, want ~now for immediate publish", got.ScheduledAt)
	}
}

func TestReviewQueueScheduleMinimalPayloadCompletesReview(t *testing.T) {
	ctx := context.Background()
	s := newValidateE2EStore(t)
	bearer, teamID, accID, _ := seedValidateE2E(t, s)
	h := validateE2EHandler(t, s)
	authorID := mustFirstUserID(t, s)

	draft, err := s.CreateScheduledPost(ctx, teamID, domain.AuthenticatedPrincipal{
		User: domain.User{ID: authorID},
		Kind: "api_token",
	}, domain.CreatePostInput{
		Title:          "RSS Review Item",
		Content:        "Draft from automation",
		ScheduledAt:    time.Now().UTC().Add(-2 * time.Hour),
		TargetAccounts: []string{accID},
		Draft:          true,
		Source:         domain.PostSourceAutomation,
	})
	if err != nil {
		t.Fatalf("CreateScheduledPost: %v", err)
	}

	// Composer save: edited content only — the item stays a draft in the review
	// queue (no scheduled_at/draft change).
	editBody, _ := json.Marshal(map[string]any{
		"title":   "Edited title",
		"content": "Edited body scheduled for the future",
	})
	editReq := httptest.NewRequest(http.MethodPatch, "/v1/teams/"+teamID+"/posts/"+draft.ID, bytes.NewReader(editBody))
	editReq.Header.Set("Authorization", "Bearer "+bearer)
	editReq.Header.Set("Content-Type", "application/json")
	editRec := httptest.NewRecorder()
	h.ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK {
		t.Fatalf("edit status = %d body=%s", editRec.Code, editRec.Body.String())
	}

	// The item stays in the review queue until the queue action completes it.
	queueReq := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/review-queue", nil)
	queueReq.Header.Set("Authorization", "Bearer "+bearer)
	queueRec := httptest.NewRecorder()
	h.ServeHTTP(queueRec, queueReq)
	if queueRec.Code != http.StatusOK {
		t.Fatalf("queue status = %d body=%s", queueRec.Code, queueRec.Body.String())
	}
	var before struct {
		Items []domain.ReviewQueueItem `json:"items"`
	}
	if err := json.Unmarshal(queueRec.Body.Bytes(), &before); err != nil {
		t.Fatal(err)
	}
	for _, item := range before.Items {
		if item.ID == draft.ID {
			goto found
		}
	}
	t.Fatal("edited draft missing from the review queue")

found:
	// Queue "Schedule" after the edit sends the minimal transition signal
	// {scheduled_at, draft:false, review_complete} — the edited title/content
	// must survive the atomic draft→pending transition.
	nextTime := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Second)
	minimal, _ := json.Marshal(map[string]any{
		"scheduled_at":    nextTime.Format(time.RFC3339),
		"draft":           false,
		"review_complete": true,
	})
	reschedReq := httptest.NewRequest(http.MethodPatch, "/v1/teams/"+teamID+"/posts/"+draft.ID, bytes.NewReader(minimal))
	reschedReq.Header.Set("Authorization", "Bearer "+bearer)
	reschedReq.Header.Set("Content-Type", "application/json")
	reschedRec := httptest.NewRecorder()
	h.ServeHTTP(reschedRec, reschedReq)
	if reschedRec.Code != http.StatusOK {
		t.Fatalf("reschedule status = %d body=%s", reschedRec.Code, reschedRec.Body.String())
	}
	got, err := s.GetScheduledPost(ctx, teamID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Edited title" || got.Content != "Edited body scheduled for the future" {
		t.Fatalf("stale partial-action overwrote content: title=%q content=%q", got.Title, got.Content)
	}
	if got.Status != domain.PostStatusPending {
		t.Fatalf("status = %s, want pending after review completion", got.Status)
	}
	if !got.ScheduledAt.UTC().Equal(nextTime) {
		t.Fatalf("scheduled_at = %v, want %v", got.ScheduledAt.UTC(), nextTime)
	}
}

func TestReviewQueuePublishNowOnTerminalPostRejected(t *testing.T) {
	ctx := context.Background()
	s := newValidateE2EStore(t)
	bearer, teamID, accID, _ := seedValidateE2E(t, s)
	h := validateE2EHandler(t, s)
	authorID := mustFirstUserID(t, s)

	post, err := s.CreateScheduledPost(ctx, teamID, domain.AuthenticatedPrincipal{
		User: domain.User{ID: authorID},
		Kind: "api_token",
	}, domain.CreatePostInput{
		Title:          "Posted",
		Content:        "Already published",
		ScheduledAt:    time.Now().UTC().Add(-1 * time.Hour),
		TargetAccounts: []string{accID},
	})
	if err != nil {
		t.Fatalf("CreateScheduledPost: %v", err)
	}
	if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{"publish_now": true})
	req := httptest.NewRequest(http.MethodPatch, "/v1/teams/"+teamID+"/posts/"+post.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s, want 409", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Error-Code"); got != "already_published" {
		t.Fatalf("X-Error-Code = %q, want already_published", got)
	}
}

func TestReviewQueueCreatePastTimeRejectedPublishNowCreates(t *testing.T) {
	s := newValidateE2EStore(t)
	bearer, teamID, accID, _ := seedValidateE2E(t, s)
	h := validateE2EHandler(t, s)

	createAndAssert := func(body map[string]any, wantStatus int) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+teamID+"/posts", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != wantStatus {
			t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), wantStatus)
		}
		return rec
	}

	// A past timestamp on a non-draft create must not silently publish.
	past := createAndAssert(map[string]any{
		"title":           "Past",
		"content":         "Must not publish",
		"scheduled_at":    time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		"target_accounts": []string{accID},
		"draft":           false,
	}, http.StatusUnprocessableEntity)
	if got := past.Header().Get("X-Error-Code"); got != "past_scheduled_at" {
		t.Fatalf("X-Error-Code = %q, want past_scheduled_at", got)
	}

	// publish_now creates a pending post scheduled at ~now with the persisted body.
	nowRec := createAndAssert(map[string]any{
		"title":           "Publish now",
		"content":         "Immediate body",
		"target_accounts": []string{accID},
		"publish_now":     true,
	}, http.StatusCreated)
	var created domain.ScheduledPost
	if err := json.Unmarshal(nowRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Status != domain.PostStatusPending {
		t.Fatalf("status = %s, want pending", created.Status)
	}
	if !created.ScheduledAt.After(time.Now().Add(-30 * time.Second)) {
		t.Fatalf("scheduled_at = %v, want ~now", created.ScheduledAt)
	}
	if created.Content != "Immediate body" {
		t.Fatalf("content = %q, want the create payload", created.Content)
	}
}

// TestReviewQueueStaleActionConflicts is the API-level race test: a composer
// save schedules the draft to a future pending state, then a stale queue
// review-transition (publish-now) arrives. The transition must 409 with
// review_conflict and leave the newer state (future timestamp + composer
// content) untouched.
func TestReviewQueueStaleActionConflicts(t *testing.T) {
	ctx := context.Background()
	s := newValidateE2EStore(t)
	bearer, teamID, accID, _ := seedValidateE2E(t, s)
	h := validateE2EHandler(t, s)
	authorID := mustFirstUserID(t, s)

	draft, err := s.CreateScheduledPost(ctx, teamID, domain.AuthenticatedPrincipal{
		User: domain.User{ID: authorID},
		Kind: "api_token",
	}, domain.CreatePostInput{
		Title:          "Review item",
		Content:        "original draft body",
		ScheduledAt:    time.Now().UTC().Add(-3 * time.Hour),
		TargetAccounts: []string{accID},
		Draft:          true,
		Source:         domain.PostSourceAutomation,
	})
	if err != nil {
		t.Fatalf("CreateScheduledPost: %v", err)
	}

	// Composer save wins: full edit scheduling the draft 48h out.
	future := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	editBody, _ := json.Marshal(map[string]any{
		"title":           "Edited title",
		"content":         "Edited body that must survive the stale queue action",
		"scheduled_at":    future.Format(time.RFC3339),
		"target_accounts": []string{accID},
		"draft":           false,
	})
	editReq := httptest.NewRequest(http.MethodPatch, "/v1/teams/"+teamID+"/posts/"+draft.ID, bytes.NewReader(editBody))
	editReq.Header.Set("Authorization", "Bearer "+bearer)
	editReq.Header.Set("Content-Type", "application/json")
	editRec := httptest.NewRecorder()
	h.ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK {
		t.Fatalf("edit status = %d body=%s", editRec.Code, editRec.Body.String())
	}

	// Stale queue publish-now after the post left draft.
	stale, _ := json.Marshal(map[string]any{"publish_now": true, "review_complete": true})
	staleReq := httptest.NewRequest(http.MethodPatch, "/v1/teams/"+teamID+"/posts/"+draft.ID, bytes.NewReader(stale))
	staleReq.Header.Set("Authorization", "Bearer "+bearer)
	staleReq.Header.Set("Content-Type", "application/json")
	staleRec := httptest.NewRecorder()
	h.ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusConflict {
		t.Fatalf("stale status = %d body=%s, want 409", staleRec.Code, staleRec.Body.String())
	}
	if got := staleRec.Header().Get("X-Error-Code"); got != "review_conflict" {
		t.Fatalf("X-Error-Code = %q, want review_conflict", got)
	}

	got, err := s.GetScheduledPost(ctx, teamID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.PostStatusPending {
		t.Fatalf("status = %s, want pending (unchanged)", got.Status)
	}
	if got.Content != "Edited body that must survive the stale queue action" {
		t.Fatalf("content = %q, want composer edit preserved", got.Content)
	}
	if d := time.Until(got.ScheduledAt); d < 40*time.Hour {
		t.Fatalf("scheduled_at = %v, want the composer's future timestamp, not ~now", got.ScheduledAt)
	}
}

// TestReviewQueueReviewCompleteExclusive rejects a review-transition that
// smuggles content changes: the queue action may only move the review item out
// of the draft state.
func TestReviewQueueReviewCompleteExclusive(t *testing.T) {
	ctx := context.Background()
	s := newValidateE2EStore(t)
	bearer, teamID, accID, _ := seedValidateE2E(t, s)
	h := validateE2EHandler(t, s)
	authorID := mustFirstUserID(t, s)

	draft, err := s.CreateScheduledPost(ctx, teamID, domain.AuthenticatedPrincipal{
		User: domain.User{ID: authorID},
		Kind: "api_token",
	}, domain.CreatePostInput{
		Title:          "Review item",
		Content:        "original",
		ScheduledAt:    time.Now().UTC().Add(-3 * time.Hour),
		TargetAccounts: []string{accID},
		Draft:          true,
		Source:         domain.PostSourceAutomation,
	})
	if err != nil {
		t.Fatalf("CreateScheduledPost: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"content":         "smuggled content change",
		"review_complete": true,
	})
	req := httptest.NewRequest(http.MethodPatch, "/v1/teams/"+teamID+"/posts/"+draft.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Error-Code"); got != "review_complete_exclusive" {
		t.Fatalf("X-Error-Code = %q, want review_complete_exclusive", got)
	}

	got, err := s.GetScheduledPost(ctx, teamID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.PostStatusDraft || got.Content != "original" {
		t.Fatalf("draft must be untouched: status=%s content=%q", got.Status, got.Content)
	}
}

func mustFirstUserID(t *testing.T, s interface {
	ListUsers(ctx context.Context) ([]domain.User, error)
}) string {
	t.Helper()
	users, err := s.ListUsers(context.Background())
	if err != nil || len(users) == 0 {
		t.Fatalf("ListUsers: %v", err)
	}
	return users[0].ID
}
