package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/sse"
)

// Scope fallback must retain #169's target validation and atomic completion claim.
func TestCompleteAIJob_ScheduleAuthority(t *testing.T) {
	for _, tc := range []struct {
		name          string
		authority     any
		foreign       bool
		overLimit     bool
		wantPost      domain.PostStatus
		wantFailure   bool
		noAutoPublish bool
		emptyContent  bool
	}{
		{name: "allowed", authority: true, wantPost: domain.PostStatusPending},
		{name: "draft only", authority: false, wantPost: domain.PostStatusDraft},
		{name: "legacy", wantPost: domain.PostStatusDraft},
		{name: "invalid authority", authority: "true", wantPost: domain.PostStatusDraft},
		{name: "draft retains long content", authority: false, overLimit: true, wantPost: domain.PostStatusDraft},
		{name: "scheduled rejects long content", authority: true, overLimit: true, wantFailure: true},
		{name: "draft rejects foreign target", authority: false, foreign: true, wantFailure: true},
		{name: "scheduled rejects foreign target", authority: true, foreign: true, wantFailure: true},
		{name: "no auto publish", authority: false, noAutoPublish: true},
		{name: "empty content", authority: false, emptyContent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newAICompletionFixture(t)
			if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: !tc.noAutoPublish}); err != nil {
				t.Fatal(err)
			}
			targetTeam := f.team.ID
			if tc.foreign {
				other, err := f.store.CreateTeam(ctx, f.user.ID, domain.CreateTeamInput{Name: "foreign"})
				if err != nil {
					t.Fatal(err)
				}
				targetTeam = other.ID
			}
			account := callbackAccount(t, f, targetTeam)
			envelope := map[string]any{"params": map[string]any{"target_account_ids": []string{account.ID}}}
			if tc.authority != nil {
				envelope["_can_schedule"] = tc.authority
			}
			payload, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			job, err := f.store.CreateAIJob(ctx, domain.AIJob{TeamID: f.team.ID, AuthorUserID: f.user.ID, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			hub := sse.NewHub()
			events, unsubscribe := hub.Subscribe(f.team.ID, "")
			defer unsubscribe()
			instance := api.New(slog.New(slog.NewTextHandler(&logs, nil)), f.store, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, hub)
			content := "generated"
			if tc.overLimit {
				content = strings.Repeat("x", 501)
			}
			if tc.emptyContent {
				content = ""
			}
			result, _ := json.Marshal(map[string]any{"content": content})
			for range 2 {
				instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, result, "")
			}
			posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
			if err != nil {
				t.Fatal(err)
			}
			updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantFailure {
				if len(posts) != 0 || updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage != "Auto-publish failed: post could not be created" {
					t.Fatalf("invalid output: posts=%d job=%+v", len(posts), updated)
				}
				if !strings.Contains(logs.String(), "ai job auto-publish failed") {
					t.Errorf("missing validation failure log: %s", logs.String())
				}
			} else if tc.wantPost == "" {
				if len(posts) != 0 || updated.Status != domain.AIJobStatusCompleted {
					t.Fatalf("unexpected post or failed completion: posts=%+v job=%+v", posts, updated)
				}
			} else {
				if len(posts) != 1 || posts[0].Status != tc.wantPost || posts[0].Content != content || updated.Status != domain.AIJobStatusCompleted {
					t.Fatalf("want one %s post with original content; posts=%+v job=%+v", tc.wantPost, posts, updated)
				}
				if tc.wantPost == domain.PostStatusDraft && !strings.Contains(logs.String(), "created draft without schedule authority") {
					t.Errorf("missing draft fallback log: %s", logs.String())
				}
			}

			var output struct {
				Content string `json:"content"`
				Notice  string `json:"notice"`
			}
			if err := json.Unmarshal(updated.Result, &output); err != nil {
				t.Fatal(err)
			}
			wantNotice := ""
			if tc.wantPost == domain.PostStatusDraft {
				wantNotice = "saved_as_draft_without_schedule_authority"
			}
			if output.Notice != wantNotice || output.Content != content {
				t.Errorf("result=%s want notice=%q and original content", updated.Result, wantNotice)
			}
			if updated.Status == domain.AIJobStatusCompleted && updated.ErrorMessage != "" {
				t.Errorf("notice must not be an error: %+v", updated)
			}
			select {
			case event := <-events:
				var streamed domain.AIJob
				if err := json.Unmarshal([]byte(event.Data), &streamed); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(streamed.Result, updated.Result) {
					t.Errorf("SSE result=%s persisted=%s", streamed.Result, updated.Result)
				}

				data, _ := json.Marshal(event)
				if !bytes.Contains(data, []byte(updated.Status)) {
					t.Errorf("SSE missing final status: %s", data)
				}
			default:
				t.Fatal("missing completion SSE")
			}
			select {
			case event := <-events:
				t.Errorf("duplicate completion SSE: %+v", event)
			default:
			}
		})
	}
}
