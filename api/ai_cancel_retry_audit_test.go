package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
)

type cancelRetryAuditStore struct {
	store.Store
	failure  string
	reads    int
	releases int
}

func (s *cancelRetryAuditStore) GetAIJobByID(ctx context.Context, teamID, jobID string) (domain.AIJob, error) {
	s.reads++
	if s.failure == "reload" && s.reads == 2 {
		return domain.AIJob{}, errors.New("private reload failure")
	}
	return s.Store.GetAIJobByID(ctx, teamID, jobID)
}

func (s *cancelRetryAuditStore) ClaimAIJobCompletion(ctx context.Context, jobID string, status domain.AIJobStatus, result []byte, message string) (bool, error) {
	if s.failure == "claim" {
		return false, errors.New("private claim failure")
	}
	return s.Store.ClaimAIJobCompletion(ctx, jobID, status, result, message)
}

func (s *cancelRetryAuditStore) ReleaseRSSItemAttempt(ctx context.Context, teamID, feedID, key string) (int, error) {
	s.releases++
	if s.failure == "release" {
		return 0, errors.New("private release failure")
	}
	return s.Store.ReleaseRSSItemAttempt(ctx, teamID, feedID, key)
}

func (s *cancelRetryAuditStore) InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	if s.failure == "audit" {
		return errors.New("private audit failure")
	}
	return s.Store.InsertAuditEvent(ctx, event)
}

func cancelRetryAuditHandler(t *testing.T, s store.Store, logs *bytes.Buffer) http.Handler {
	t.Helper()
	authSvc, err := auth.New(context.Background(), config.Config{}, s)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	return api.New(logger, s, authSvc, provider.NewRegistry(), config.Config{}, nil, catalog, nil, nil).Handler(security.NewLimiter(10000, 10000), nil)
}

func makeCancelRSSAttempt(t *testing.T, f aiCancelFixture, attempts int) (domain.RSSFeedConfig, domain.AIJob) {
	t.Helper()
	ctx := context.Background()
	feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/cancel-retry", Name: "cancel-retry"})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := range attempts {
		if attempt > 0 {
			if _, err := f.store.ReleaseRSSItemAttempt(ctx, f.team.ID, feed.ID, "item"); err != nil {
				t.Fatal(err)
			}
		}
		if claimed, err := f.store.ClaimRSSItemAttempt(ctx, f.team.ID, feed.ID, "item"); err != nil || !claimed {
			t.Fatalf("reserve attempt %d: claimed=%v err=%v", attempt+1, claimed, err)
		}
	}
	payload, err := json.Marshal(map[string]any{"params": map[string]any{"rss_automation": map[string]string{"feed_id": feed.ID, "item_key": "item"}}})
	if err != nil {
		t.Fatal(err)
	}
	job, err := f.store.CreateAIJob(ctx, domain.AIJob{TeamID: f.team.ID, AuthorUserID: f.user.ID, Type: domain.AIJobTypeVoiceEngine, Origin: domain.AIJobOriginAutomation, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return feed, job
}

func TestCancelRSSAuditFailureRollsBackRetry(t *testing.T) {
	ctx := context.Background()
	f := newAICancelFixture(t)
	feed, job := makeCancelRSSAttempt(t, f, 1)
	wrapped := &cancelRetryAuditStore{Store: f.store, failure: "audit"}
	var logs bytes.Buffer
	h := cancelRetryAuditHandler(t, wrapped, &logs)
	url := "/v1/teams/" + f.team.ID + "/ai-jobs/" + job.ID + "/cancel"
	rec := doRequest(t, h, http.MethodPost, url, f.bearer, nil)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "private audit failure") {
		t.Fatalf("audit failure response: %d %s", rec.Code, rec.Body.String())
	}
	current, err := f.store.GetAIJobByID(ctx, f.team.ID, job.ID)
	if err != nil || current.Status != domain.AIJobStatusPending || current.CompletedAt != nil {
		t.Fatalf("failed audit committed cancellation: %+v err=%v", current, err)
	}
	if claimed, err := f.store.ClaimRSSItemAttempt(ctx, f.team.ID, feed.ID, "item"); err != nil || claimed {
		t.Fatalf("failed audit released active attempt: claimed=%v err=%v", claimed, err)
	}
	if count, err := f.store.CountAuditEvents(ctx, domain.AuditFilter{TeamID: f.team.ID, Action: "ai_job.cancel"}); err != nil || count != 0 {
		t.Fatalf("rolled back evidence count=%d err=%v", count, err)
	}
	// Retry after audit recovery must release the original attempt successfully.
	wrapped.failure = ""
	rec = doRequest(t, h, http.MethodPost, url, f.bearer, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("retry cancellation: %d %s", rec.Code, rec.Body.String())
	}
	if claimed, err := f.store.ClaimRSSItemAttempt(ctx, f.team.ID, feed.ID, "item"); err != nil || !claimed {
		t.Fatalf("successful retry did not release item: claimed=%v err=%v", claimed, err)
	}
}

func TestCancelRSSRepeatedRequestWritesOneAuditAndRelease(t *testing.T) {
	ctx := context.Background()
	f := newAICancelFixture(t)
	feed, job := makeCancelRSSAttempt(t, f, 1)
	wrapped := &cancelRetryAuditStore{Store: f.store}
	var logs bytes.Buffer
	h := cancelRetryAuditHandler(t, wrapped, &logs)
	url := "/v1/teams/" + f.team.ID + "/ai-jobs/" + job.ID + "/cancel"
	for range 2 {
		rec := doRequest(t, h, http.MethodPost, url, f.bearer, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
		}
	}
	if wrapped.releases != 1 {
		t.Fatalf("duplicate cancellation released item %d times", wrapped.releases)
	}
	events, err := f.store.ListAuditEvents(ctx, domain.AuditFilter{TeamID: f.team.ID, Action: "ai_job.cancel"})
	if err != nil || len(events) != 1 {
		t.Fatalf("cancel evidence=%+v err=%v", events, err)
	}
	if events[0].ActorKind != domain.AuditActorToken || events[0].ActorUserID != f.user.ID || events[0].TargetID == nil || *events[0].TargetID != job.ID {
		t.Fatalf("cancellation attribution lost: %+v", events[0])
	}
	if claimed, err := f.store.ClaimRSSItemAttempt(ctx, f.team.ID, feed.ID, "item"); err != nil || !claimed {
		t.Fatalf("second attempt unavailable: claimed=%v err=%v", claimed, err)
	}
	// A late repeated cancel must not release a newer attempt for the same item.
	rec := doRequest(t, h, http.MethodPost, url, f.bearer, nil)
	if rec.Code != http.StatusOK || wrapped.releases != 1 {
		t.Fatalf("late cancellation replay: status=%d releases=%d", rec.Code, wrapped.releases)
	}
	if claimed, err := f.store.ClaimRSSItemAttempt(ctx, f.team.ID, feed.ID, "item"); err != nil || claimed {
		t.Fatalf("late cancellation released newer attempt: claimed=%v err=%v", claimed, err)
	}
}

func TestCancelRSSPersistenceFailuresLeaveRetryReserved(t *testing.T) {
	for _, scenario := range []string{"claim", "reload", "conflict_reload"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			f := newAICancelFixture(t)
			feed, job := makeCancelRSSAttempt(t, f, 1)
			failure := scenario
			wantStatus := domain.AIJobStatusPending
			if scenario == "conflict_reload" {
				failure = "reload"
				wantStatus = domain.AIJobStatusCompleted
				if _, err := f.store.ClaimAIJobCompletion(ctx, job.ID, wantStatus, []byte(`{"content":"winner"}`), ""); err != nil {
					t.Fatal(err)
				}
			}
			wrapped := &cancelRetryAuditStore{Store: f.store, failure: failure}
			var logs bytes.Buffer
			h := cancelRetryAuditHandler(t, wrapped, &logs)
			rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+job.ID+"/cancel", f.bearer, nil)
			if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "private") {
				t.Fatalf("persistence failure response: %d %s", rec.Code, rec.Body.String())
			}
			current, err := f.store.GetAIJobByID(ctx, f.team.ID, job.ID)
			if err != nil || current.Status != wantStatus || current.ErrorMessage != "" {
				t.Fatalf("failed request changed job: %+v err=%v", current, err)
			}
			if claimed, err := f.store.ClaimRSSItemAttempt(ctx, f.team.ID, feed.ID, "item"); err != nil || claimed {
				t.Fatalf("failed request released item: claimed=%v err=%v", claimed, err)
			}
			if count, err := f.store.CountAuditEvents(ctx, domain.AuditFilter{TeamID: f.team.ID, Action: "ai_job.cancel"}); err != nil || count != 0 {
				t.Fatalf("failed request audit count=%d err=%v", count, err)
			}
			if !strings.Contains(logs.String(), "private") {
				t.Fatalf("failure details absent from logs: %s", logs.String())
			}
		})
	}
}

func TestCancelRSSRetryReleaseWarnings(t *testing.T) {
	for _, scenario := range []string{"exhausted", "release_failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			f := newAICancelFixture(t)
			attempts := domain.MaxRSSItemAttempts
			failure, warning := "", "cancelled RSS AI job exhausted item attempts"
			if scenario == "release_failure" {
				attempts, failure, warning = 1, "release", "cancelled RSS AI job item release failed"
			}
			feed, job := makeCancelRSSAttempt(t, f, attempts)
			wrapped := &cancelRetryAuditStore{Store: f.store, failure: failure}
			var logs bytes.Buffer
			h := cancelRetryAuditHandler(t, wrapped, &logs)
			rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+job.ID+"/cancel", f.bearer, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(logs.String(), "item_key=") {
				t.Fatal("cancel warning exposes raw RSS item key")
			}
			for _, field := range []string{"level=WARN", warning, "job_id=" + job.ID, "feed_id=" + feed.ID} {
				if !strings.Contains(logs.String(), field) {
					t.Fatalf("release warning lacks %q: %s", field, logs.String())
				}
			}
			if claimed, err := f.store.ClaimRSSItemAttempt(ctx, f.team.ID, feed.ID, "item"); err != nil || claimed {
				t.Fatalf("exhausted/unreleased item reclaimed: claimed=%v err=%v", claimed, err)
			}
			if scenario == "exhausted" {
				current, err := f.store.GetRSSFeedConfigByID(ctx, f.team.ID, feed.ID)
				if err != nil || current.ExhaustedItemsCount != 1 {
					t.Fatalf("exhaustion hidden from feed API: count=%d err=%v", current.ExhaustedItemsCount, err)
				}
			}
		})
	}
}
