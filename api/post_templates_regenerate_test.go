package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/scheduler"
	"git.f4mily.net/goloom/internal/security"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

type regenerateStub struct {
	horizonFn func(ctx context.Context, teamID, templateID string) (domain.PostTemplateRegenerateResult, error)
}

func (s *regenerateStub) RegeneratePostTemplateOccurrence(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) (domain.PostTemplateRegenerateResult, error) {
	return domain.PostTemplateRegenerateResult{}, nil
}

func (s *regenerateStub) RegeneratePostTemplateHorizon(ctx context.Context, teamID, templateID string) (domain.PostTemplateRegenerateResult, error) {
	if s.horizonFn != nil {
		return s.horizonFn(ctx, teamID, templateID)
	}
	return domain.PostTemplateRegenerateResult{}, nil
}

func (s *regenerateStub) SyncPostMetricsNow(ctx context.Context)    {}
func (s *regenerateStub) SyncAccountMetricsNow(ctx context.Context) {}
func (s *regenerateStub) SyncExternalPostsNow(ctx context.Context)  {}
func (s *regenerateStub) SyncRSSFeedsNow(ctx context.Context)       {}
func (s *regenerateStub) ImportOldPosts(ctx context.Context, teamID string, input scheduler.ImportOldPostsInput) (scheduler.ImportOldPostsResult, error) {
	return scheduler.ImportOldPostsResult{}, nil
}

func TestPostTemplates_regenerateHorizon(t *testing.T) {
	s := newValidationMemoryStore(t)
	a := newTestAPI(t, s)
	a.metricsSync = &regenerateStub{
		horizonFn: func(ctx context.Context, teamID, templateID string) (domain.PostTemplateRegenerateResult, error) {
			return domain.PostTemplateRegenerateResult{DeletedPosts: 2, RegeneratedOccurrences: 2}, nil
		},
	}

	ctx := context.Background()
	u, team, acc := seedRegenerateTemplateFixtures(t, s, ctx)
	tmpl := createRegenerateTestTemplate(t, a, u, team, acc)

	body, _ := json.Marshal(map[string]any{"mode": "horizon"})
	req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+team.ID+"/post-templates/"+tmpl.ID+"/regenerate", bytes.NewReader(body))
	req.SetPathValue("teamID", team.ID)
	req.SetPathValue("templateID", tmpl.ID)
	req = req.WithContext(security.WithPrincipal(ctx, domain.AuthenticatedPrincipal{User: u}))
	rec := httptest.NewRecorder()
	a.handleRegeneratePostTemplate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out domain.PostTemplateRegenerateResult
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.DeletedPosts != 2 || out.RegeneratedOccurrences != 2 {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func seedRegenerateTemplateFixtures(t *testing.T, s *sqlitestore.Store, ctx context.Context) (domain.User, domain.Team, domain.SocialAccount) {
	t.Helper()
	u, err := s.UpsertOIDCUser(ctx, "regen-"+uuid.NewString(), "regen@test.test", "Regen")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "regen-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://mastodon.test", Username: "regen", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	return u, team, acc
}

func createRegenerateTestTemplate(t *testing.T, a *API, u domain.User, team domain.Team, acc domain.SocialAccount) domain.PostTemplate {
	t.Helper()
	recJSON := `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`
	body, _ := json.Marshal(map[string]any{
		"title":              "Weekly",
		"content":            "Hello",
		"recurrence_json":    recJSON,
		"target_account_ids": []string{acc.ID},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+team.ID+"/post-templates", bytes.NewReader(body))
	req.SetPathValue("teamID", team.ID)
	req = req.WithContext(security.WithPrincipal(context.Background(), domain.AuthenticatedPrincipal{User: u}))
	rec := httptest.NewRecorder()
	a.handleCreatePostTemplate(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", rec.Code, rec.Body.String())
	}
	var tmpl domain.PostTemplate
	if err := json.NewDecoder(rec.Body).Decode(&tmpl); err != nil {
		t.Fatal(err)
	}
	return tmpl
}
