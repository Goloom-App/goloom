package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/scheduler"
	"git.f4mily.net/goloom/internal/store"
)

func TestRSSInsertFailureReleasesItem(t *testing.T) {
	ctx := context.Background()
	f, account, _ := newRecurringCallbackFixture(t)
	feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "retry", TargetAccountIDs: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.RecordRSSImportedItem(ctx, feed.ID, "item", ""); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"params": map[string]any{
		"target_account_ids": []string{account.ID},
		"rss_automation":     map[string]any{"feed_id": feed.ID, "item_key": "item", "output_mode": "draft", "fallback_content": "fallback"},
	}})
	job := makeRecurringJob(t, f, payload)
	failing := &automationWriteFailureStore{Store: f.store, operation: "insert"}
	instance := api.New(nil, failing, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, nil)
	instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"generated"}`), "")
	imported, err := f.store.RSSItemAlreadyImported(ctx, feed.ID, "item")
	if err != nil {
		t.Fatal(err)
	}
	if imported {
		t.Fatal("failed post insert left RSS item imported; next poll cannot retry")
	}
}

type rssRetryRunner struct{}

func (rssRetryRunner) RunJob(context.Context, domain.AIJob, domain.AIServiceConfig, domain.AIContext) (json.RawMessage, error) {
	return json.RawMessage(`{"content":"generated"}`), nil
}

func TestRSSPollRetriesInsertFailureOnceAndBoundsPermanentFailure(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(fmt.Sprint("permanent=", permanent), func(t *testing.T) {
			ctx := context.Background()
			f, account, _ := newRecurringCallbackFixture(t)
			feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>retry</title><link>https://example.test</link><description>test</description><item><title>item</title><guid>retry-item</guid><link>https://example.test/item</link><pubDate>%s</pubDate><description>body</description></item></channel></rss>`, time.Now().UTC().Add(-time.Minute).Format(time.RFC1123Z))
			}))
			defer feedServer.Close()
			feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{FeedURL: feedServer.URL, Name: "retry", IsActive: true, AiEnhanceEnabled: true, OutputMode: domain.AutomationOutputDraft, ContentTemplate: "{title} {link}", TargetAccountIDs: []string{account.ID}, MaxPostsPerDay: 10})
			if err != nil {
				t.Fatal(err)
			}
			if err := f.store.UpdateRSSFeedLastFetched(ctx, feed.ID, time.Now().UTC().Add(-time.Hour)); err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.UpsertAIServiceConfig(ctx, f.team.ID, domain.AIServiceConfig{Provider: "openai", APIKey: "test-key"}); err != nil {
				t.Fatal(err)
			}
			failing := &automationWriteFailureStore{Store: f.store, operation: "insert"}
			registry := provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}))
			var logs rssRetryLog
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			instance := api.New(logger, failing, nil, registry, config.Config{}, nil, nil, nil, nil)
			manager := aijobs.NewManager(failing, rssRetryRunner{})
			manager.SetCompleter(instance)
			service := scheduler.New(logger, failing, registry, time.Minute, 1, time.Minute, time.Minute, time.Minute, time.Minute, manager)
			service.SyncRSSFeedsNow(ctx)
			manager.Wait()
			if failing.inserts != 1 {
				t.Fatalf("first poll inserts=%d want=1 logs=%s", failing.inserts, logs.String())
			}
			if !permanent {
				failing.operation = ""
			}
			for range 5 {
				service.SyncRSSFeedsNow(ctx)
				manager.Wait()
			}
			posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantAttempts, wantPosts := 2, 1
			if permanent {
				wantAttempts, wantPosts = 3, 0
				if !strings.Contains(logs.String(), "exhausted=true") {
					t.Fatalf("retry exhaustion invisible: %s", logs.String())
				}
			}
			if failing.inserts != wantAttempts || len(posts) != wantPosts {
				t.Fatalf("attempts=%d posts=%d want=%d/%d", failing.inserts, len(posts), wantAttempts, wantPosts)
			}
		})
	}
}

// A store can commit successfully then fail while rereading the post for its response.
type rssPostReadFailureStore struct{ store.Store }

func (s rssPostReadFailureStore) CreateScheduledPost(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
	if _, err := s.Store.CreateScheduledPost(ctx, teamID, principal, input); err != nil {
		return domain.ScheduledPost{}, err
	}
	return domain.ScheduledPost{}, fmt.Errorf("injected post-commit read failure")
}

func TestRSSPostCommitReadFailureKeepsItemReserved(t *testing.T) {
	ctx := context.Background()
	f, account, _ := newRecurringCallbackFixture(t)
	feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "test", TargetAccountIDs: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.RecordRSSImportedItem(ctx, feed.ID, "item", ""); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"params": map[string]any{"target_account_ids": []string{account.ID}, "rss_automation": map[string]any{"feed_id": feed.ID, "item_key": "item", "output_mode": "draft", "fallback_content": "fallback"}}})
	job := makeRecurringJob(t, f, payload)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	instance := api.New(logger, rssPostReadFailureStore{f.store}, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, nil)
	instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"generated"}`), "")
	if !strings.Contains(logs.String(), `level=INFO msg="rss automation: item already linked or unavailable"`) {
		t.Fatalf("benign release missing info log: %s", logs.String())
	}
	if strings.Contains(logs.String(), `level=ERROR msg="rss automation: release item failed"`) {
		t.Fatalf("benign release logged ERROR: %s", logs.String())
	}
	imported, err := f.store.RSSItemAlreadyImported(ctx, feed.ID, "item")
	if err != nil || !imported {
		t.Fatalf("committed post released item: imported=%v err=%v", imported, err)
	}
	after, err := f.store.GetRSSFeedConfigByID(ctx, f.team.ID, feed.ID)
	if err != nil || after.CounterNext != feed.CounterNext+1 {
		t.Fatalf("post-commit error lost counter: got=%d want=%d err=%v", after.CounterNext, feed.CounterNext+1, err)
	}
	posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts=%d err=%v", len(posts), err)
	}
}

func TestRSSValidationFallbackLinkFailureKeepsItemReserved(t *testing.T) {
	ctx := context.Background()
	f, account, _ := newRecurringCallbackFixture(t)
	feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "test", TargetAccountIDs: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.RecordRSSImportedItem(ctx, feed.ID, "item", ""); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"params": map[string]any{"target_account_ids": []string{account.ID}, "rss_automation": map[string]any{"feed_id": feed.ID, "item_key": "item", "output_mode": "scheduled", "fallback_content": "fallback"}}})
	job := makeRecurringJob(t, f, payload)
	instance := api.New(nil, &automationWriteFailureStore{Store: f.store, operation: "link"}, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, nil)
	raw, _ := json.Marshal(map[string]string{"content": strings.Repeat("x", 501)})
	instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, raw, "")
	imported, err := f.store.RSSItemAlreadyImported(ctx, feed.ID, "item")
	if err != nil || !imported {
		t.Fatalf("fallback post released item: imported=%v err=%v", imported, err)
	}
	posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts=%d err=%v", len(posts), err)
	}
}

// Scheduler dispatch and asynchronous completion may log concurrently.
type rssRetryLog struct {
	sync.Mutex
	buffer bytes.Buffer
}

func (b *rssRetryLog) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.buffer.Write(p)
}
func (b *rssRetryLog) String() string { b.Lock(); defer b.Unlock(); return b.buffer.String() }
