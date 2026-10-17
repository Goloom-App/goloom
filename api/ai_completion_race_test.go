package api_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store"
)

// Force both callers to read the unfinished job before either can claim it.
type concurrentCompletionStore struct {
	store.Store
	reads atomic.Int32
	ready chan struct{}
}

func (s *concurrentCompletionStore) GetAIJobByIDGlobal(ctx context.Context, id string) (domain.AIJob, error) {
	job, err := s.Store.GetAIJobByIDGlobal(ctx, id)
	if err != nil {
		return job, err
	}
	n := s.reads.Add(1)
	if n == 2 {
		close(s.ready)
	}
	if n <= 2 {
		select {
		case <-s.ready:
		case <-ctx.Done():
			return domain.AIJob{}, ctx.Err()
		}
	}
	return job, nil
}

func TestConcurrentAICompletionCreatesOnePost(t *testing.T) {
	for _, scenario := range []string{"auto-publish", "automation-success", "automation-fallback"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			f, account, template := newRecurringCallbackFixture(t)
			var job domain.AIJob
			var err error
			if scenario == "auto-publish" {
				if _, err = f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: true}); err != nil {
					t.Fatal(err)
				}
				payload, _ := json.Marshal(map[string]any{"params": map[string]any{"target_account_ids": []string{account.ID}}})
				job, err = f.store.CreateAIJob(ctx, domain.AIJob{TeamID: f.team.ID, AuthorUserID: f.user.ID, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				job = makeRecurringJob(t, f, recurringJobPayload(template.ID, account.ID, "fallback", time.Now().UTC().Add(time.Hour)))
			}
			s := &concurrentCompletionStore{Store: f.store, ready: make(chan struct{})}
			// Independent API instances share only the database, as separate replicas do.
			var wg sync.WaitGroup
			for range 2 {
				instance := api.New(nil, s, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, nil)
				wg.Go(func() {
					if scenario == "automation-fallback" {
						instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusFailed, nil, "provider failed")
					} else {
						instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"generated"}`), "")
					}
				})
			}
			wg.Wait()
			if err := ctx.Err(); err != nil {
				t.Fatal(err)
			}
			posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(posts) != 1 {
				t.Fatalf("concurrent completions created %d posts, want exactly one", len(posts))
			}
			want := "generated"
			if scenario == "automation-fallback" {
				want = "fallback"
			}
			if posts[0].Content != want {
				t.Errorf("post content=%q want=%q", posts[0].Content, want)
			}
		})
	}
}
