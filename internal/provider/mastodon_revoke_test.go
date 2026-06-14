package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestMastodonRevokeAccessToken(t *testing.T) {
	t.Parallel()
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/revoke" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	p, ok := NewMastodonProvider(MastodonRegistrationConfig{}).(OAuthTokenRevoker)
	if !ok {
		t.Fatal("mastodon provider does not implement OAuthTokenRevoker")
	}
	instance := domain.ProviderInstance{InstanceURL: srv.URL, ClientID: "cid"}
	if err := p.RevokeAccessToken(context.Background(), instance, "secret", "access-tok"); err != nil {
		t.Fatalf("RevokeAccessToken: %v", err)
	}
	if gotForm.Get("client_id") != "cid" || gotForm.Get("client_secret") != "secret" || gotForm.Get("token") != "access-tok" {
		t.Fatalf("unexpected form values: %v", gotForm)
	}
}

func TestMastodonRevokeAccessToken_errorStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()

	p := NewMastodonProvider(MastodonRegistrationConfig{}).(OAuthTokenRevoker)
	instance := domain.ProviderInstance{InstanceURL: srv.URL, ClientID: "cid"}
	if err := p.RevokeAccessToken(context.Background(), instance, "secret", "access-tok"); err == nil {
		t.Fatal("expected error on non-2xx status")
	}
}
