package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
)

type auditOAuthProvider struct{ provider.SocialMediaProvider }

func (auditOAuthProvider) Name() string { return "mastodon" }
func (auditOAuthProvider) BuildAuthorizationURL(domain.ProviderInstance, string, string) (string, error) {
	return "", nil
}
func (auditOAuthProvider) ConnectAccountOAuthCallback(context.Context, domain.ProviderInstance, string, string, string) (domain.ConnectedAccount, error) {
	return domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, InstanceURL: "https://provider.test", Username: "account", AccessToken: "NEVER-AUDIT"}, nil
}
func TestOAuthAuditUsesValidatedStateActor(t *testing.T) {
	s := newValidationMemoryStore(t)
	a := newTestAPI(t, s)
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "oauth-actor", "actor@test.test", "Actor")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "OAuth"})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := s.CreateProviderInstance(ctx, u.ID, domain.PreparedProviderInstance{Provider: "mastodon", Name: "instance", InstanceURL: "https://provider.test", ClientID: "client", ClientSecret: "NEVER-AUDIT"})
	if err != nil {
		t.Fatal(err)
	}
	a.config.PublicBaseURL = "https://app.test"
	a.providers = provider.NewRegistry(auditOAuthProvider{})
	signed, err := a.signMastodonOAuthState(mastodonOAuthState{Version: 1, Provider: "mastodon", UserID: u.ID, TeamID: team.ID, ProviderInstanceID: instance.ID, ReturnTo: "https://app.test/", ExpiresAtUnix: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	request := func(state string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/oauth/mastodon/callback?code=NEVER-AUDIT&state="+url.QueryEscape(state), nil)
		w := httptest.NewRecorder()
		a.handleMastodonOAuthCallback(w, req)
		return w
	}
	if w := request(signed + "tampered"); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid state accepted: %d", w.Code)
	}
	if w := request(signed); w.Code != http.StatusSeeOther {
		t.Fatalf("callback: %d %s", w.Code, w.Body.String())
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID})
	if err != nil || len(events) != 1 {
		t.Fatalf("events: %+v %v", events, err)
	}
	e := events[0]
	if e.ActorUserID != u.ID || e.ActorKind != domain.AuditActorHuman || e.TokenID != nil || e.Metadata["channel"] != "oauth" || e.TargetID == nil {
		t.Fatalf("callback attribution: %+v", e)
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "NEVER-AUDIT") {
		t.Fatalf("secret leaked: %s", raw)
	}
}
