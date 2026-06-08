package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func TestAIContext(t *testing.T) {
	t.Run("ReturnsAggregatedData", func(t *testing.T) {
		ctx := context.Background()
		s := newAICRUDStore(t)
		h := newAICRUDHandler(t, s)

		u, err := s.UpsertOIDCUser(ctx, "ai-context-"+uuid.NewString(), "ctx@example.test", "AI Context")
		if err != nil {
			t.Fatal(err)
		}
		team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ctx-team-" + uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		enabled := true
		if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
			t.Fatal(err)
		}

		profile := domain.TeamProfile{
			TeamID: team.ID,
			StyleMetadata: domain.StyleMetadata{
				Tonality:          "professional",
				FormattingRules:   []string{"short sentences"},
				BannedWords:       []string{"spam"},
				MaxHashtags:       3,
				PreferredLanguage: "en",
			},
			AutoPublishEnabled: false,
		}
		if _, err := s.CreateTeamProfile(ctx, team.ID, profile); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateCampaignFormat(ctx, team.ID, domain.CampaignFormat{
			TeamID:           team.ID,
			Name:             "Weekly roundup",
			Structure:        json.RawMessage(`{"sections":["intro","body"]}`),
			RequiredHashtags: []string{"#weekly"},
			IsActive:         true,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateStyleExample(ctx, team.ID, domain.StyleExample{
			TeamID:   team.ID,
			Platform: "mastodon",
			Content:  "Keep it concise.",
			Notes:    "sample",
		}); err != nil {
			t.Fatal(err)
		}
		longContent := strings.Repeat("0123456789", 32)
		post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: u}, domain.CreatePostInput{
			Content:     longContent,
			ScheduledAt: time.Now().UTC(),
			Draft:       false,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
			t.Fatal(err)
		}

		scopes, _ := json.Marshal([]string{auth.ScopeAIReadContext})
		bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "ctx-token", nil, string(scopes), nil)
		if err != nil {
			t.Fatal(err)
		}

		rec := doRequest(t, h, http.MethodGet, "/v1/teams/"+team.ID+"/ai-context", bearer, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET ai-context: got %d; body: %s", rec.Code, rec.Body.String())
		}

		var got domain.AIContext
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.Team.ID != team.ID {
			t.Fatalf("team id = %q, want %q", got.Team.ID, team.ID)
		}
		if got.Profile == nil || got.Profile.TeamID != team.ID {
			t.Fatalf("profile missing or wrong team")
		}
		if len(got.CampaignFormats) != 1 {
			t.Fatalf("campaign formats = %d, want 1", len(got.CampaignFormats))
		}
		if len(got.StyleExamples) != 1 {
			t.Fatalf("style examples = %d, want 1", len(got.StyleExamples))
		}
		if len(got.RecentPosts) != 1 {
			t.Fatalf("recent posts = %d, want 1", len(got.RecentPosts))
		}
		if len(got.RecentPosts[0].Content) != 280 {
			t.Fatalf("trimmed content length = %d, want 280", len(got.RecentPosts[0].Content))
		}
		if got.RecentPosts[0].Content != longContent[len(longContent)-280:] {
			t.Fatalf("trimmed content mismatch")
		}
	})
}
