package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"github.com/google/uuid"
)

func TestPostTemplates_createListUpdate(t *testing.T) {
	s := newValidationMemoryStore(t)
	a := newTestAPI(t, s)
	ctx := context.Background()

	u, err := s.UpsertOIDCUser(ctx, "pt-"+uuid.NewString(), "pt@test.test", "PT")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "pt-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://mastodon.test", Username: "pt", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"Europe/Berlin"}`
	createBody, _ := json.Marshal(map[string]any{
		"title":                 "Weekly post",
		"content":               "Hello {counter}",
		"recurrence_json":       rec,
		"target_account_ids":    []string{acc.ID},
		"enabled":               true,
		"materialize_horizon_days": 21,
		"announcement_enabled":  true,
		"announcement_title":    "Soon",
		"announcement_content":  "Event on {main_day}.{main_month}",
		"announcement_days_before": 2,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/v1/teams/"+team.ID+"/post-templates", bytes.NewReader(createBody))
	createReq.SetPathValue("teamID", team.ID)
	createReq = createReq.WithContext(security.WithPrincipal(ctx, domain.AuthenticatedPrincipal{User: u}))

	createRec := httptest.NewRecorder()
	a.handleCreatePostTemplate(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", createRec.Code, createRec.Body.String())
	}
	var created domain.PostTemplate
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !created.AnnouncementEnabled || created.MaterializeHorizonDays != 21 {
		t.Fatalf("unexpected create result: %+v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/teams/"+team.ID+"/post-templates", nil)
	listReq.SetPathValue("teamID", team.ID)
	listRec := httptest.NewRecorder()
	a.handleListPostTemplates(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Items []domain.PostTemplate `json:"items"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Items) != 1 || listResp.Items[0].ID != created.ID {
		t.Fatalf("list items: %+v", listResp.Items)
	}

	patchBody, _ := json.Marshal(map[string]any{
		"content":                  "Updated {counter}",
		"materialize_horizon_days": 14,
	})
	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/teams/"+team.ID+"/post-templates/"+created.ID, bytes.NewReader(patchBody))
	patchReq.SetPathValue("teamID", team.ID)
	patchReq.SetPathValue("templateID", created.ID)
	patchRec := httptest.NewRecorder()
	a.handleUpdatePostTemplate(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("update status %d: %s", patchRec.Code, patchRec.Body.String())
	}
	var updated domain.PostTemplate
	if err := json.NewDecoder(patchRec.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Content != "Updated {counter}" || updated.MaterializeHorizonDays != 14 {
		t.Fatalf("unexpected update: %+v", updated)
	}
}
