package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func TestAIDraft(t *testing.T) {
	t.Run("CreatesDraftPost", func(t *testing.T) {
		ctx := context.Background()
		s := newAICRUDStore(t)
		h := newAICRUDHandler(t, s)

		u, err := s.UpsertOIDCUser(ctx, "ai-draft-"+uuid.NewString(), "draft@example.test", "AI Draft")
		if err != nil {
			t.Fatal(err)
		}
		team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "draft-team-" + uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		enabled := true
		if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
			t.Fatal(err)
		}
		acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
			Provider:        "mastodon",
			AuthType:        domain.AccountAuthTypeOAuthToken,
			InstanceURL:     "https://mastodon.social",
			Username:        "draft-user",
			RemoteAccountID: "acct-1",
			AccessToken:     "tok",
		})
		if err != nil {
			t.Fatal(err)
		}
		scopes, _ := json.Marshal([]string{auth.ScopeWriteDraft})
		bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "draft-token", nil, string(scopes), nil, "")
		if err != nil {
			t.Fatal(err)
		}

		body := map[string]any{
			"content":     "draft body",
			"account_ids": []string{acc.ID},
			"ai_job_id":   "job-1",
			"metadata":    map[string]any{"source": "ai"},
		}
		rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+team.ID+"/posts/draft", bearer, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST draft: got %d; body: %s", rec.Code, rec.Body.String())
		}

		var got domain.ScheduledPost
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.Status != domain.PostStatusDraft {
			t.Fatalf("status = %q, want draft", got.Status)
		}
		if got.AuthorUserID != u.ID {
			t.Fatalf("author_user_id = %q, want %q", got.AuthorUserID, u.ID)
		}
		if len(got.TargetAccounts) != 1 || got.TargetAccounts[0] != acc.ID {
			t.Fatalf("target_accounts = %#v, want [%s]", got.TargetAccounts, acc.ID)
		}
	})

	t.Run("AutoPublishCreatesScheduled", func(t *testing.T) {
		ctx := context.Background()
		s := newAICRUDStore(t)
		h := newAICRUDHandler(t, s)

		u, err := s.UpsertOIDCUser(ctx, "ai-draft-auto-"+uuid.NewString(), "auto@example.test", "AI Draft Auto")
		if err != nil {
			t.Fatal(err)
		}
		team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "auto-team-" + uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		enabled := true
		if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateTeamProfile(ctx, team.ID, domain.TeamProfile{
			TeamID:             team.ID,
			StyleMetadata:      domain.StyleMetadata{Tonality: "professional", FormattingRules: []string{"clear"}, MaxHashtags: 2, PreferredLanguage: "en"},
			AutoPublishEnabled: true,
		}); err != nil {
			t.Fatal(err)
		}
		acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
			Provider:        "mastodon",
			AuthType:        domain.AccountAuthTypeOAuthToken,
			InstanceURL:     "https://mastodon.social",
			Username:        "auto-user",
			RemoteAccountID: "acct-2",
			AccessToken:     "tok",
		})
		if err != nil {
			t.Fatal(err)
		}
		scopes, _ := json.Marshal([]string{auth.ScopeWriteDraft, auth.ScopeWriteSchedule})
		bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "draft-token-auto", nil, string(scopes), nil, "")
		if err != nil {
			t.Fatal(err)
		}

		scheduledAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		body := map[string]any{
			"content":      "publish later",
			"account_ids":  []string{acc.ID},
			"scheduled_at": scheduledAt.Format(time.RFC3339),
			"ai_job_id":    "job-2",
		}
		rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+team.ID+"/posts/draft", bearer, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST draft auto-publish: got %d; body: %s", rec.Code, rec.Body.String())
		}

		var got domain.ScheduledPost
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.Status != domain.PostStatusPending {
			t.Fatalf("status = %q, want pending", got.Status)
		}
		if !got.ScheduledAt.Equal(scheduledAt) {
			t.Fatalf("scheduled_at = %v, want %v", got.ScheduledAt, scheduledAt)
		}
	})
}

func TestAIDraft_ScheduleScope(t *testing.T) {
	for _, tc := range []struct {
		name        string
		scopes      []string
		schedule    bool
		autoPublish bool
		regularPost bool
		wantCode    int
		wantStatus  domain.PostStatus
	}{
		{name: "draft only stays draft", scopes: []string{auth.ScopeWriteDraft}, wantCode: http.StatusCreated, wantStatus: domain.PostStatusDraft},
		{name: "draft only cannot schedule", scopes: []string{auth.ScopeWriteDraft}, schedule: true, wantCode: http.StatusForbidden},
		{name: "draft only cannot auto publish", scopes: []string{auth.ScopeWriteDraft}, autoPublish: true, wantCode: http.StatusForbidden},
		{name: "regular post control", scopes: []string{auth.ScopeWriteDraft}, regularPost: true, wantCode: http.StatusForbidden},
		{name: "schedule scope can schedule", scopes: []string{auth.ScopeWriteDraft, auth.ScopeWriteSchedule}, schedule: true, wantCode: http.StatusCreated, wantStatus: domain.PostStatusPending},
		{name: "write can auto publish", scopes: []string{auth.ScopeWrite}, autoPublish: true, wantCode: http.StatusCreated, wantStatus: domain.PostStatusPending},
		{name: "unscoped can schedule", schedule: true, wantCode: http.StatusCreated, wantStatus: domain.PostStatusPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newAITriggerFixture(t, nil, tc.scopes...)
			if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: tc.autoPublish}); err != nil {
				t.Fatal(err)
			}
			account, err := f.store.CreateAccount(ctx, f.team.ID, domain.ConnectedAccount{
				Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
				InstanceURL: "https://mastodon.social", Username: "draft-scope", AccessToken: "tok",
			})
			if err != nil {
				t.Fatal(err)
			}
			path := "/v1/teams/" + f.team.ID + "/posts/draft"
			body := map[string]any{
				"content": "scope-checked AI post", "account_ids": []string{account.ID},
				"scheduled_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "schedule": tc.schedule,
			}
			if tc.regularPost {
				path = "/v1/teams/" + f.team.ID + "/posts"
				body["draft"] = false
			}
			rec := doRequest(t, f.h, http.MethodPost, path, f.bearer, body)
			if rec.Code != tc.wantCode {
				t.Fatalf("POST: got %d, want %d; body: %s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.wantCode == http.StatusForbidden {
				if rec.Header().Get("X-Error-Code") != "scope_required" {
					t.Fatalf("expected scope_required; body: %s", rec.Body.String())
				}
				posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
				if err != nil || len(posts) != 0 {
					t.Fatalf("forbidden request created posts: %v, error: %v", posts, err)
				}
				return
			}
			var got domain.ScheduledPost
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, tc.wantStatus)
			}
		})
	}
}

func TestAIDraft_RejectsCrossTeamTarget(t *testing.T) {
	ctx := context.Background()
	s := newAICRUDStore(t)
	h := newAICRUDHandler(t, s)

	u, err := s.UpsertOIDCUser(ctx, "ai-draft-xteam-"+uuid.NewString(), "xteam@example.test", "AI Draft XTeam")
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	teamA, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "a-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTeam(ctx, teamA.ID, domain.UpdateTeamInput{Name: teamA.Name, IsAIEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	teamB, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "b-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	accB, err := s.CreateAccount(ctx, teamB.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://mastodon.social", Username: "b-user", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	scopes, _ := json.Marshal([]string{auth.ScopeWriteDraft})
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "xteam-token", nil, string(scopes), nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// The user owns both teams, but a draft for team A must not target team B's account.
	body := map[string]any{
		"content":     "draft body",
		"account_ids": []string{accB.ID},
	}
	rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+teamA.ID+"/posts/draft", bearer, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cross-team AI draft: got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}
