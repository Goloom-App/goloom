package api_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

type aiCompletionFixture struct {
	store *sqlitestore.Store
	api   *api.API
	team  domain.Team
	user  domain.User
}

func newAICompletionFixture(t *testing.T) aiCompletionFixture {
	t.Helper()
	ctx := context.Background()
	s := newAICRUDStore(t)

	authSvc, err := auth.New(ctx, config.Config{}, s)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	reg := provider.NewRegistry(
		provider.NewBlueskyProvider(),
		provider.NewFriendicaProvider(),
		provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}),
	)
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatalf("i18n.Load: %v", err)
	}
	apiInstance := api.New(nil, s, authSvc, reg, config.Config{}, nil, catalog, nil, nil)

	u, err := s.UpsertOIDCUser(ctx, "ai-cb-"+uuid.NewString(), "callback@example.test", "AI Callback")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "cb-team-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	return aiCompletionFixture{store: s, api: apiInstance, team: team, user: u}
}

func makeCallbackJob(t *testing.T, f aiCompletionFixture, status domain.AIJobStatus) domain.AIJob {
	t.Helper()
	job, err := f.store.CreateAIJob(context.Background(), domain.AIJob{
		TeamID:       f.team.ID,
		AuthorUserID: f.user.ID,
		Type:         domain.AIJobTypeVoiceEngine,
		Status:       status,
		Payload:      json.RawMessage(`{"params":{}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestCompleteAIJob(t *testing.T) {
	t.Run("UpdatesStatusNoAutoDraft", func(t *testing.T) {
		ctx := context.Background()
		f := newAICompletionFixture(t)
		job := makeCallbackJob(t, f, domain.AIJobStatusPending)

		f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"hello"}`), "")

		updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetAIJobByIDGlobal: %v", err)
		}
		if updated.Status != domain.AIJobStatusCompleted {
			t.Fatalf("job status = %q, want %q", updated.Status, domain.AIJobStatusCompleted)
		}

		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil {
			t.Fatalf("ListTeamPosts: %v", err)
		}
		if len(posts) != 0 {
			t.Fatalf("expected 0 auto-created posts (auto_publish_enabled=false), got %d", len(posts))
		}
	})

	t.Run("AutoCreatesPostOnCompletion", func(t *testing.T) {
		ctx := context.Background()
		f := newAICompletionFixture(t)

		if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{
			TeamID:             f.team.ID,
			AutoPublishEnabled: true,
			StyleMetadata:      domain.StyleMetadata{Tonality: "casual"},
		}); err != nil {
			t.Fatal(err)
		}

		job := makeCallbackJob(t, f, domain.AIJobStatusPending)
		scheduledAt := time.Date(2027, 1, 15, 12, 0, 0, 0, time.UTC)
		result, err := json.Marshal(map[string]any{
			"content":      "AI-generated post content",
			"scheduled_at": scheduledAt.Format(time.RFC3339),
		})
		if err != nil {
			t.Fatal(err)
		}

		f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, result, "")

		updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetAIJobByIDGlobal: %v", err)
		}
		if updated.Status != domain.AIJobStatusCompleted {
			t.Fatalf("job status = %q, want %q", updated.Status, domain.AIJobStatusCompleted)
		}

		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil {
			t.Fatalf("ListTeamPosts: %v", err)
		}
		if len(posts) != 1 {
			t.Fatalf("expected 1 auto-created post, got %d", len(posts))
		}
		if posts[0].Content != "AI-generated post content" {
			t.Fatalf("post content = %q, want %q", posts[0].Content, "AI-generated post content")
		}
		if posts[0].Status != domain.PostStatusPending {
			t.Fatalf("post status = %q, want %q", posts[0].Status, domain.PostStatusPending)
		}
	})

	t.Run("IdempotentForTerminalJob", func(t *testing.T) {
		ctx := context.Background()
		f := newAICompletionFixture(t)

		if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{
			TeamID:             f.team.ID,
			AutoPublishEnabled: true,
			StyleMetadata:      domain.StyleMetadata{Tonality: "casual"},
		}); err != nil {
			t.Fatal(err)
		}

		job := makeCallbackJob(t, f, domain.AIJobStatusCompleted)

		f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"duplicate"}`), "")

		refetched, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetAIJobByIDGlobal: %v", err)
		}
		if refetched.Status != domain.AIJobStatusCompleted {
			t.Fatalf("job status changed unexpectedly: got %q", refetched.Status)
		}

		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil {
			t.Fatalf("ListTeamPosts: %v", err)
		}
		if len(posts) != 0 {
			t.Fatalf("terminal job must not trigger side effects, got %d posts", len(posts))
		}
	})

	t.Run("MarksJobFailed", func(t *testing.T) {
		ctx := context.Background()
		f := newAICompletionFixture(t)
		job := makeCallbackJob(t, f, domain.AIJobStatusPending)

		f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusFailed, nil, "provider exploded")

		updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetAIJobByIDGlobal: %v", err)
		}
		if updated.Status != domain.AIJobStatusFailed {
			t.Fatalf("job status = %q, want %q", updated.Status, domain.AIJobStatusFailed)
		}
		if updated.ErrorMessage != "provider exploded" {
			t.Fatalf("error message = %q", updated.ErrorMessage)
		}
	})
}
