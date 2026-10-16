package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/sse"
	"git.f4mily.net/goloom/internal/store"
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

	// Burn the first-user admin slot before creating the ordinary job author.
	if _, err := s.UpsertOIDCUser(ctx, "bootstrap-"+uuid.NewString(), "admin@example.test", "Admin"); err != nil {
		t.Fatal(err)
	}
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

		account := callbackAccount(t, f, f.team.ID)
		payload, _ := json.Marshal(map[string]any{"params": map[string]any{"target_account_ids": []string{account.ID}}})
		job, err := f.store.CreateAIJob(ctx, domain.AIJob{TeamID: f.team.ID, AuthorUserID: f.user.ID, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
		if err != nil {
			t.Fatal(err)
		}
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

func callbackAccount(t *testing.T, f aiCompletionFixture, teamID string) domain.SocialAccount {
	t.Helper()
	account, err := f.store.CreateAccount(context.Background(), teamID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://mastodon.social", Username: uuid.NewString(), RemoteAccountID: uuid.NewString(), AccessToken: "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func TestCompleteAIJobAutoPublishTargets(t *testing.T) {
	for _, scenario := range []string{"foreign", "mixed", "unknown", "same-team-versions"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			f := newAICompletionFixture(t)
			if f.user.IsAdmin {
				t.Fatal("job author must not be admin")
			}
			if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: true}); err != nil {
				t.Fatal(err)
			}
			// Separate owner: the job author has no membership in the foreign team.
			owner, err := f.store.UpsertOIDCUser(ctx, uuid.NewString(), "foreign@test", "Foreign")
			if err != nil {
				t.Fatal(err)
			}
			other, err := f.store.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "foreign"})
			if err != nil {
				t.Fatal(err)
			}
			own := callbackAccount(t, f, f.team.ID)
			foreign := callbackAccount(t, f, other.ID)
			targets := []string{foreign.ID}
			switch scenario {
			case "mixed":
				targets = []string{own.ID, foreign.ID}
			case "unknown":
				targets = []string{uuid.NewString()}
			case "same-team-versions":
				targets = []string{own.ID}
			}
			payload, _ := json.Marshal(map[string]any{"params": map[string]any{"target_account_ids": targets}})
			job, err := f.store.CreateAIJob(ctx, domain.AIJob{TeamID: f.team.ID, AuthorUserID: f.user.ID, Type: domain.AIJobTypeCampaignAutopilot, Payload: payload})
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			hub := sse.NewHub()
			events, unsubscribe := hub.Subscribe(f.team.ID, "")
			defer unsubscribe()
			f.api = api.New(slog.New(slog.NewTextHandler(&logs, nil)), f.store, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, hub)
			result, _ := json.Marshal(map[string]any{"content": "generated", "account_content_override": map[string]string{targets[0]: "account version"}})
			f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, result, "")
			updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "same-team-versions" {
				if updated.Status != domain.AIJobStatusCompleted || len(posts) != 1 {
					t.Fatalf("status=%s posts=%d", updated.Status, len(posts))
				}
				if posts[0].Status != domain.PostStatusPending || len(posts[0].TargetAccounts) != 1 || posts[0].TargetAccounts[0] != own.ID {
					t.Fatalf("post: %+v", posts[0])
				}
				versions, err := f.store.ListPostVersionsForTeamPost(ctx, f.team.ID, posts[0].ID)
				if err != nil {
					t.Fatal(err)
				}
				if len(versions) != 1 || versions[0].AccountID != own.ID || versions[0].Content != "account version" {
					t.Fatalf("versions: %+v", versions)
				}
				return
			}
			if updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage == "" {
				t.Errorf("invalid targets: status=%s error=%q; want failed with error", updated.Status, updated.ErrorMessage)
			}
			if len(posts) != 0 {
				t.Errorf("invalid targets created %d publishable posts", len(posts))
			}
			if !strings.Contains(logs.String(), "ERROR") {
				t.Errorf("missing error log: %s", logs.String())
			}
			select {
			case event := <-events:
				data, _ := json.Marshal(event)
				if !bytes.Contains(data, []byte("failed")) {
					t.Errorf("SSE event lacks failure: %s", data)
				}
			case <-time.After(time.Second):
				t.Fatal("missing completion SSE")
			}
		})
	}
}

type failingAutoPostStore struct{ store.Store }

type failOnceAIStatusStore struct {
	store.Store
	failed bool
}

func (s *failOnceAIStatusStore) UpdateAIJobStatus(ctx context.Context, id string, status domain.AIJobStatus, result []byte, message string) error {
	if !s.failed {
		s.failed = true
		return errors.New("injected status write failure")
	}
	return s.Store.UpdateAIJobStatus(ctx, id, status, result, message)
}

func TestCompleteAIJobStatusWriteFailureRetry(t *testing.T) {
	ctx := context.Background()
	f := newAICompletionFixture(t)
	if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: true}); err != nil {
		t.Fatal(err)
	}
	account := callbackAccount(t, f, f.team.ID)
	payload, _ := json.Marshal(map[string]any{"params": map[string]any{"target_account_ids": []string{account.ID}}})
	job, err := f.store.CreateAIJob(ctx, domain.AIJob{TeamID: f.team.ID, AuthorUserID: f.user.ID, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	instance := api.New(nil, &failOnceAIStatusStore{Store: f.store}, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, nil)
	result := json.RawMessage(`{"content":"generated"}`)
	instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, result, "")
	posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Errorf("failed status write created %d posts", len(posts))
	}
	for range 2 {
		instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, result, "")
	}
	posts, err = f.store.ListTeamPosts(ctx, f.team.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("retry: posts=%d err=%v; want exactly one", len(posts), err)
	}
}

func (s failingAutoPostStore) CreateScheduledPost(context.Context, string, domain.AuthenticatedPrincipal, domain.CreatePostInput) (domain.ScheduledPost, error) {
	return domain.ScheduledPost{}, errors.New("injected post insert failure")
}

func TestCompleteAIJobAutoPublishInsertFailure(t *testing.T) {
	ctx := context.Background()
	f := newAICompletionFixture(t)
	if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: true}); err != nil {
		t.Fatal(err)
	}
	account := callbackAccount(t, f, f.team.ID)
	payload, _ := json.Marshal(map[string]any{"params": map[string]any{"target_account_ids": []string{account.ID}}})
	job, err := f.store.CreateAIJob(ctx, domain.AIJob{TeamID: f.team.ID, AuthorUserID: f.user.ID, Type: domain.AIJobTypeCampaignAutopilot, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	instance := api.New(slog.New(slog.NewTextHandler(&logs, nil)), failingAutoPostStore{f.store}, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, nil)
	instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"generated"}`), "")
	updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage == "" {
		t.Errorf("status=%s error=%q; want failed with error", updated.Status, updated.ErrorMessage)
	}
	if !strings.Contains(logs.String(), "injected post insert failure") {
		t.Errorf("missing insert failure log: %s", logs.String())
	}
	posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("posts=%v err=%v", posts, err)
	}
}
