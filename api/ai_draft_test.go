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
		scopes, _ := json.Marshal([]string{auth.ScopeAIWriteDrafts})
		bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "draft-token", nil, string(scopes), nil)
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
		scopes, _ := json.Marshal([]string{auth.ScopeAIWriteDrafts})
		bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "draft-token-auto", nil, string(scopes), nil)
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
