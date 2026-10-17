package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func TestTokenPrivilegeRoutesRequireFullAuthority(t *testing.T) {
	type action struct {
		method, path string
		body         any
		status       int
		snapshot     string
		global       bool
	}
	cases := []struct {
		name  string
		setup func(*testing.T, endpointFixture) action
	}{
		{"add owner", func(t *testing.T, f endpointFixture) action {
			user, err := f.store.UpsertOIDCUser(context.Background(), uuid.NewString(), "new-owner@example.test", "New owner")
			if err != nil {
				t.Fatal(err)
			}
			path := "/v1/teams/" + f.team.ID + "/members"
			return action{http.MethodPost, path, map[string]any{"user_id": user.ID, "role": "owner"}, http.StatusCreated, path, false}
		}},
		{"create invitation", func(t *testing.T, f endpointFixture) action {
			path := "/v1/teams/" + f.team.ID + "/invitations"
			return action{http.MethodPost, path, map[string]any{"email": "invited@example.test", "role": "editor"}, http.StatusCreated, path, false}
		}},
		{"revoke invitation", func(t *testing.T, f endpointFixture) action {
			path := "/v1/teams/" + f.team.ID + "/invitations"
			rec := f.do(t, http.MethodPost, path, map[string]any{"email": "invited@example.test", "role": "editor"})
			requireStatus(t, rec, http.StatusCreated)
			created := decodeJSON[struct {
				Invitation domain.TeamInvitation `json:"invitation"`
			}](t, rec)
			return action{http.MethodDelete, path + "/" + created.Invitation.ID, nil, http.StatusNoContent, path, false}
		}},
		{"promote admin", func(t *testing.T, f endpointFixture) action {
			user, err := f.store.UpsertOIDCUser(context.Background(), uuid.NewString(), "promoted@example.test", "Promoted")
			if err != nil {
				t.Fatal(err)
			}
			if user.IsAdmin {
				t.Fatal("target already admin")
			}
			return action{http.MethodPatch, "/v1/admin/users/" + user.ID, map[string]any{"is_admin": true}, http.StatusOK, "/v1/admin/users", true}
		}},
		{"create team", func(t *testing.T, f endpointFixture) action {
			return action{http.MethodPost, "/v1/teams", map[string]any{"name": "New team"}, http.StatusCreated, "/v1/teams", true}
		}},
		{"patch team", func(t *testing.T, f endpointFixture) action {
			return action{http.MethodPatch, "/v1/teams/" + f.team.ID, map[string]any{"name": "Renamed team"}, http.StatusOK, "/v1/teams", false}
		}},
		{"AI service config", func(t *testing.T, f endpointFixture) action {
			path := "/v1/teams/" + f.team.ID + "/ai-service-config"
			return action{http.MethodPut, path, map[string]any{"provider": "openai", "model": "test", "api_key": "sk-secret", "base_url": "https://api.openai.com/v1"}, http.StatusOK, path, false}
		}},
		{"migrate account", func(t *testing.T, f endpointFixture) action {
			team, err := f.store.CreateTeam(context.Background(), f.user.ID, domain.CreateTeamInput{Name: "Destination"})
			if err != nil {
				t.Fatal(err)
			}
			base := "/v1/teams/" + f.team.ID + "/accounts"
			return action{http.MethodPost, base + "/" + f.account.ID + "/migrate", map[string]any{"target_team_id": team.ID}, http.StatusOK, base, true}
		}},
	}
	for _, tc := range cases {
		for _, authority := range []string{"read", "write and delete", "unscoped", "own team", "other team"} {
			t.Run(tc.name+"/"+authority, func(t *testing.T) {
				f := newEndpointFixture(t)
				a := tc.setup(t, f)
				unrestricted := f.bearer
				var scopes string
				var binding *string
				switch authority {
				case "read":
					scopes = `["read"]`
				case "write and delete":
					scopes = `["write","delete"]`
				case "own team":
					binding = &f.team.ID
				case "other team":
					team, err := f.store.CreateTeam(context.Background(), f.user.ID, domain.CreateTeamInput{Name: "Other"})
					if err != nil {
						t.Fatal(err)
					}
					binding = &team.ID
				}
				before := f.do(t, http.MethodGet, a.snapshot, nil)
				token, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, "privilege test", nil, scopes, binding, "")
				if err != nil {
					t.Fatal(err)
				}
				f.bearer = token
				rec := f.do(t, a.method, a.path, a.body)
				allowed := authority == "unscoped" || authority == "own team" && !a.global
				if allowed {
					requireStatus(t, rec, a.status)
				} else {
					requireStatus(t, rec, http.StatusForbidden)
					if authority != "other team" && rec.Header().Get("X-Error-Code") != "scope_required" && !strings.Contains(rec.Body.String(), "scope_required") {
						t.Fatalf("missing scope_required: %s", rec.Body.String())
					}
				}
				f.bearer = unrestricted
				after := f.do(t, http.MethodGet, a.snapshot, nil)
				changed := before.Code != after.Code || before.Body.String() != after.Body.String()
				if changed != allowed {
					t.Fatalf("state changed=%v; allowed=%v; before=%s after=%s", changed, allowed, before.Body.String(), after.Body.String())
				}
			})
		}
	}
}

func TestPromptPreviewScopeAndTeamStatePrivacy(t *testing.T) {
	f := newEndpointFixture(t)
	path := "/v1/teams/" + f.team.ID + "/ai/prompt-preview"
	body := map[string]any{"params": map[string]any{"target_account_ids": []string{f.account.ID}, "schedule": false}}
	for _, scopes := range []string{`["write:draft"]`, `["read"]`} {
		t.Run(scopes, func(t *testing.T) {
			token, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, "preview", nil, scopes, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			f.bearer = token
			rec := f.do(t, http.MethodPost, path, body)
			if scopes == `["read"]` {
				requireStatus(t, rec, http.StatusOK)
			} else {
				requireStatus(t, rec, http.StatusForbidden)
				if rec.Header().Get("X-Error-Code") != "scope_required" && !strings.Contains(rec.Body.String(), "scope_required") {
					t.Fatalf("missing scope_required: %s", rec.Body.String())
				}
			}
		})
	}
	t.Run("team state privacy", func(t *testing.T) {
		outsider, err := f.store.UpsertOIDCUser(context.Background(), uuid.NewString(), "outsider@example.test", "Outsider")
		if err != nil {
			t.Fatal(err)
		}
		f.bearer, _, err = f.store.CreateUserAPIToken(context.Background(), outsider.ID, "outsider", nil, `["read"]`, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		var previous string
		for _, enabled := range []bool{true, false} {
			if _, err := f.store.UpdateTeam(context.Background(), f.team.ID, domain.UpdateTeamInput{Name: f.team.Name, IsAIEnabled: &enabled}); err != nil {
				t.Fatal(err)
			}
			rec := f.do(t, http.MethodPost, path, body)
			requireStatus(t, rec, http.StatusForbidden)
			if rec.Header().Get("X-Error-Code") != "forbidden" && strings.TrimSpace(rec.Body.String()) != "forbidden" {
				t.Fatalf("team state exposed: %s", rec.Body.String())
			}
			if previous != "" && previous != rec.Body.String() {
				t.Fatalf("team state changes response: %q vs %q", previous, rec.Body.String())
			}
			previous = rec.Body.String()
		}
	})
}
