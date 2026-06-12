package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestRegistry(t *testing.T) {
	reg := NewRegistry(
		NewBlueskyProvider(),
		NewFriendicaProvider(),
		NewMastodonProvider(MastodonRegistrationConfig{}),
	)
	if _, ok := reg.Get("bluesky"); !ok {
		t.Fatal("bluesky missing")
	}
	if _, ok := reg.Get("Mastodon"); !ok {
		t.Fatal("lookup must be case-insensitive")
	}
	if _, ok := reg.Get("does-not-exist"); ok {
		t.Fatal("unknown provider must not resolve")
	}
	supported := reg.Supported()
	if len(supported) != 3 || supported[0] != "bluesky" || supported[1] != "friendica" || supported[2] != "mastodon" {
		t.Fatalf("supported = %v", supported)
	}
}

func TestFriendicaPublish(t *testing.T) {
	var gotPath, gotAuth string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		_ = json.NewEncoder(w).Encode(mastodonStatusResponse{
			ID: "42", URL: server_url(r) + "/display/42", URI: "urn:42",
		})
	}))
	defer server.Close()

	p := NewFriendicaProvider()
	account := domain.SocialAccount{Provider: "friendica", InstanceURL: server.URL}
	result, err := p.Publish(context.Background(), account, PublishAuth{AccessToken: "tok-1"}, PublishRequest{
		Content: "Hallo Fediverse", Visibility: "public",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if gotPath != "/api/v1/statuses" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer tok-1" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotPayload["status"] != "Hallo Fediverse" {
		t.Fatalf("payload = %v", gotPayload)
	}
	if result.RemoteID != "42" || result.Metadata["uri"] != "urn:42" {
		t.Fatalf("result = %+v", result)
	}
}

// server_url reconstructs the test server base URL from the request.
func server_url(r *http.Request) string {
	return "http://" + r.Host
}

func TestFriendicaPublishServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewFriendicaProvider()
	_, err := p.Publish(context.Background(), domain.SocialAccount{InstanceURL: server.URL}, PublishAuth{AccessToken: "t"}, PublishRequest{Content: "x"})
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("expected publish failure with status, got %v", err)
	}
}

func TestFriendicaConnectAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/accounts/verify_credentials" {
			_ = json.NewEncoder(w).Encode(map[string]string{"avatar": "https://cdn.example/a.png"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := NewFriendicaProvider()
	// The SSRF guard blocks loopback addresses; allow it for the test server.
	ctx := WithOutboundInstancePolicy(context.Background(), OutboundPolicy{AllowPrivateLAN: true})
	acc, err := p.ConnectAccount(ctx, domain.CreateAccountInput{
		InstanceURL: server.URL, Username: "tester", AccessToken: "tok",
	}, nil)
	if err != nil {
		t.Fatalf("ConnectAccount: %v", err)
	}
	if acc.Provider != "friendica" || acc.Username != "tester" {
		t.Fatalf("account = %+v", acc)
	}
	if acc.AvatarURL != "https://cdn.example/a.png" {
		t.Fatalf("avatar = %q", acc.AvatarURL)
	}

	if _, err := p.ConnectAccount(ctx, domain.CreateAccountInput{
		InstanceURL: server.URL, Username: "", AccessToken: "",
	}, nil); err == nil {
		t.Fatal("missing credentials must error")
	}

	// Without the dev policy the loopback test server must be rejected (SSRF guard).
	if _, err := p.ConnectAccount(context.Background(), domain.CreateAccountInput{
		InstanceURL: server.URL, Username: "tester", AccessToken: "tok",
	}, nil); err == nil {
		t.Fatal("loopback instance must be rejected without AllowPrivateLAN")
	}
}

func TestBlueskyPublishWithAppPassword(t *testing.T) {
	var sessionCalls, createCalls int
	var recordAuth string
	var createBody struct {
		Repo       string         `json:"repo"`
		Collection string         `json:"collection"`
		Record     map[string]any `json:"record"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			sessionCalls++
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["identifier"] != "tester.bsky" || body["password"] != "app-pass" {
				t.Errorf("session body = %v", body)
			}
			_ = json.NewEncoder(w).Encode(blueskySessionResponse{DID: "did:plc:x", AccessJWT: "jwt-123"})
		case "/xrpc/com.atproto.repo.createRecord":
			createCalls++
			recordAuth = r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&createBody)
			_ = json.NewEncoder(w).Encode(blueskyCreateRecordResponse{URI: "at://did:plc:x/app.bsky.feed.post/abc", CID: "cid1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := NewBlueskyProvider().(*BlueskyProvider)
	account := domain.SocialAccount{
		Provider: "bluesky", InstanceURL: server.URL,
		Username: "tester.bsky", RemoteAccountID: "did:plc:x",
		AuthType: domain.AccountAuthTypeAppPassword,
	}
	result, err := p.Publish(context.Background(), account, PublishAuth{AccessToken: "app-pass"}, PublishRequest{
		Content: "Hallo @tester.bsky https://example.org #golang",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if sessionCalls != 1 || createCalls != 1 {
		t.Fatalf("calls: session=%d create=%d", sessionCalls, createCalls)
	}
	if recordAuth != "Bearer jwt-123" {
		t.Fatalf("record must use session JWT, got %q", recordAuth)
	}
	if createBody.Repo != "did:plc:x" || createBody.Collection != "app.bsky.feed.post" {
		t.Fatalf("create body: %+v", createBody)
	}
	if _, ok := createBody.Record["facets"]; !ok {
		t.Fatal("links/tags in content must produce facets")
	}
	if result.RemoteID == "" || !strings.Contains(result.URL, "bsky.app/profile/") {
		t.Fatalf("result = %+v", result)
	}
}

func TestBlueskyPublishErrors(t *testing.T) {
	p := NewBlueskyProvider().(*BlueskyProvider)

	if _, err := p.Publish(context.Background(), domain.SocialAccount{}, PublishAuth{}, PublishRequest{Content: "x"}); err == nil {
		t.Fatal("missing credential must error")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer server.Close()
	account := domain.SocialAccount{InstanceURL: server.URL, RemoteAccountID: "did:x", AuthType: domain.AccountAuthTypeOAuthToken}

	if _, err := p.Publish(context.Background(), account, PublishAuth{AccessToken: "jwt"}, PublishRequest{Content: "x"}); err == nil || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("expected status 400 error, got %v", err)
	}

	if _, err := p.Publish(context.Background(), account, PublishAuth{AccessToken: "jwt"}, PublishRequest{Content: "   "}); err == nil {
		t.Fatal("empty content without media must error")
	}
}

func TestMastodonPublishSendsVisibilityAndCW(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/statuses" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		_ = json.NewEncoder(w).Encode(mastodonStatusResponse{ID: "7", URL: "https://m.example/@u/7"})
	}))
	defer server.Close()

	p := NewMastodonProvider(MastodonRegistrationConfig{})
	account := domain.SocialAccount{Provider: "mastodon", InstanceURL: server.URL}
	result, err := p.Publish(context.Background(), account, PublishAuth{AccessToken: "tok"}, PublishRequest{
		Content: "Achtung", Visibility: "unlisted", SpoilerText: "CW", Sensitive: true,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if gotPayload["visibility"] != "unlisted" || gotPayload["spoiler_text"] != "CW" || gotPayload["sensitive"] != true {
		t.Fatalf("payload = %v", gotPayload)
	}
	if result.RemoteID != "7" {
		t.Fatalf("result = %+v", result)
	}
}

func TestProviderCapabilities(t *testing.T) {
	ctx := context.Background()
	account := domain.SocialAccount{}

	caps, err := NewBlueskyProvider().Capabilities(ctx, account)
	if err != nil || caps.MaxChars != 300 {
		t.Fatalf("bluesky caps = %+v err=%v", caps, err)
	}
	caps, err = NewFriendicaProvider().Capabilities(ctx, account)
	if err != nil || caps.MaxChars <= 0 {
		t.Fatalf("friendica caps = %+v err=%v", caps, err)
	}
	caps, err = NewMastodonProvider(MastodonRegistrationConfig{}).Capabilities(ctx, account)
	if err != nil || caps.MaxChars != 500 {
		t.Fatalf("mastodon caps = %+v err=%v", caps, err)
	}
}
