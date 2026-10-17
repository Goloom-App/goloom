package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/rss"
	"git.f4mily.net/goloom/internal/store"
)

type submissionFailureStore struct{ store.Store }

func (s submissionFailureStore) GetAIServiceConfig(context.Context, string) (domain.AIServiceConfig, error) {
	return domain.AIServiceConfig{}, errors.New("https://credential:secret@example.test/?token=secret")
}

func TestDirectAISubmissionFallbackIsAudited(t *testing.T) {
	for _, kind := range []string{"rss", domain.TemplatePostRoleMain, domain.TemplatePostRoleAnnouncement} {
		for _, enhance := range []bool{true, false} {
			name := kind + "/submission_failure"
			if !enhance {
				name = kind + "/ai_disabled"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				s, _, team, user := newRecurringAIFixture(t, true)
				account, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, AccessToken: "secret"})
				if err != nil {
					t.Fatal(err)
				}
				wrapped := submissionFailureStore{s}
				manager := aijobs.NewManager(wrapped, noopRunner{})
				svc := New(testLogger(), wrapped, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
				occurrence := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
				sourceID := ""
				if kind == "rss" {
					feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{Name: "Feed", FeedURL: "https://example.test/feed", AiEnhanceEnabled: enhance, OutputMode: domain.AutomationOutputDraft, TargetAccountIDs: []string{account.ID}})
					if err != nil {
						t.Fatal(err)
					}
					sourceID = feed.ID
					created, err := svc.createPostFromRSSItem(ctx, feed, rss.Item{GUID: "item", Title: "source body", Content: "source body", Link: "https://example.test/item", PublishedAt: occurrence})
					if err != nil || !created {
						t.Fatalf("RSS fallback: created=%v err=%v", created, err)
					}
				} else {
					tmpl, err := s.CreatePostTemplate(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostTemplateInput{Title: "source body", Content: "source body", TargetAccountIDs: []string{account.ID}, RecurrenceJSON: `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`, AiEnhanceEnabled: &enhance, AiEnhanceAnnouncement: &enhance, AnnouncementEnabled: boolPtr(true), AnnouncementTitle: "source body", AnnouncementContent: "source body"})
					if err != nil {
						t.Fatal(err)
					}
					sourceID = tmpl.ID
					if kind == domain.TemplatePostRoleMain {
						err = svc.createScheduledPostFromTemplate(ctx, &tmpl, occurrence, occurrence, kind)
					} else {
						err = svc.materializeAnnouncement(ctx, &tmpl, occurrence, true)
					}
					if err != nil {
						t.Fatalf("recurring fallback: %v", err)
					}
				}
				manager.Wait()
				posts, err := s.ListTeamPosts(ctx, team.ID)
				if err != nil || len(posts) != 1 {
					t.Fatalf("resulting posts=%d err=%v; want one template post", len(posts), err)
				}
				jobs, err := s.ListAIJobs(ctx, team.ID, 100)
				if err != nil || len(jobs) != 0 {
					t.Fatalf("failed submission left jobs: %+v %v", jobs, err)
				}
				events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.automation_create"})
				if err != nil || len(events) != 1 {
					t.Fatalf("creation evidence: %+v %v", events, err)
				}
				event := events[0]
				outcome := "success"
				code := ""
				if enhance {
					outcome = "fallback"
					code = "ai_submission_failed"
				}
				if event.Metadata["outcome"] != outcome || event.Metadata["error_code"] != code {
					t.Fatalf("fallback outcome lost: %+v", event)
				}
				if event.ActorKind != domain.AuditActorSystem || event.Metadata["channel"] != "scheduler" || event.Metadata["author_user_id"] != user.ID || event.TargetID == nil || *event.TargetID != posts[0].ID {
					t.Fatalf("fallback attribution lost: %+v", event)
				}
				if kind == "rss" {
					if event.Metadata["feed_id"] != sourceID {
						t.Fatalf("RSS source lost: %+v", event)
					}
				} else if event.Metadata["template_id"] != sourceID || event.Metadata["post_kind"] != kind {
					t.Fatalf("recurring source lost: %+v", event)
				}
				raw, _ := json.Marshal(event)
				for _, secret := range []string{"credential", "token=secret", "source body"} {
					if strings.Contains(string(raw), secret) {
						t.Fatalf("fallback evidence leaked %q: %s", secret, raw)
					}
				}
			})
		}
	}
}

type rssReservationFailureStore struct {
	store.Store
	failed atomic.Bool
}

func (s *rssReservationFailureStore) RecordRSSImportedItem(ctx context.Context, feedID, itemKey, postID string) error {
	if postID == "" && !s.failed.Swap(true) {
		return errors.New("injected RSS reservation failure")
	}
	return s.Store.RecordRSSImportedItem(ctx, feedID, itemKey, postID)
}

type submissionCountingRunner struct{ calls atomic.Int64 }

func (r *submissionCountingRunner) RunJob(context.Context, domain.AIJob, domain.AIServiceConfig, domain.AIContext) (json.RawMessage, error) {
	r.calls.Add(1)
	return json.RawMessage(`{"content":"AI result"}`), nil
}
func TestFailedRSSReservationRollsBackJobBeforeTemplateFallback(t *testing.T) {
	ctx := context.Background()
	s, _, team, _ := newRecurringAIFixture(t, true)
	account, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, AccessToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{Name: "Feed", FeedURL: "https://example.test/feed", AiEnhanceEnabled: true, OutputMode: domain.AutomationOutputDraft, TargetAccountIDs: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &rssReservationFailureStore{Store: s}
	runner := &submissionCountingRunner{}
	manager := aijobs.NewManager(wrapped, runner)
	svc := New(testLogger(), wrapped, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
	item := rss.Item{GUID: "item", Title: "Item", Content: "source", Link: "https://example.test/item", PublishedAt: time.Now().UTC()}
	created, err := svc.createPostFromRSSItem(ctx, feed, item)
	manager.Wait()
	if err != nil || !created {
		t.Fatalf("reservation fallback = %v %v", created, err)
	}
	jobs, err := s.ListAIJobs(ctx, team.ID, 100)
	if err != nil || len(jobs) != 0 || runner.calls.Load() != 0 {
		t.Fatalf("failed source reservation dispatched orphan AI: jobs=%d calls=%d err=%v", len(jobs), runner.calls.Load(), err)
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("fallback posts=%d err=%v", len(posts), err)
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID})
	if err != nil || len(events) != 1 || events[0].Action != "post.automation_create" || events[0].Metadata["outcome"] != "fallback" || events[0].Metadata["error_code"] != "ai_submission_failed" {
		t.Fatalf("fallback evidence: %+v %v", events, err)
	}
	imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, rss.ItemKey(item))
	if err != nil || !imported {
		t.Fatalf("fallback source reservation missing: %v %v", imported, err)
	}
	if replay, err := svc.createPostFromRSSItem(ctx, feed, item); err != nil || replay {
		t.Fatalf("fallback replay = %v %v", replay, err)
	}
}
