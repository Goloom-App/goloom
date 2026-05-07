package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestMastodonProvider_Publish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check path
		if r.URL.Path != "/api/v1/statuses" {
			t.Errorf("expected path /api/v1/statuses, got %s", r.URL.Path)
		}
		// Check method
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Check auth
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", auth)
		}

		// Decode body to verify content
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["status"] != "hello world" {
			t.Errorf("expected status 'hello world', got %v", body["status"])
		}

		// Respond with success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mastodonStatusResponse{
			ID:  "12345",
			URL: "https://mastodon.example/12345",
		})
	}))
	defer server.Close()

	p := NewMastodonProvider(MastodonRegistrationConfig{})
	account := domain.SocialAccount{
		InstanceURL: server.URL,
	}
	auth := PublishAuth{AccessToken: "test-token"}
	req := PublishRequest{Content: "hello world"}

	result, err := p.Publish(context.Background(), account, auth, req)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if result.RemoteID != "12345" {
		t.Errorf("expected RemoteID 12345, got %s", result.RemoteID)
	}
	if result.URL != "https://mastodon.example/12345" {
		t.Errorf("expected URL https://mastodon.example/12345, got %s", result.URL)
	}
}

func TestBlueskyProvider_Publish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check path
		if r.URL.Path != "/xrpc/com.atproto.repo.createRecord" {
			t.Errorf("expected path /xrpc/com.atproto.repo.createRecord, got %s", r.URL.Path)
		}
		// Check method
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Check auth
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", auth)
		}

		// Decode body to verify content
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		record := body["record"].(map[string]any)
		if record["text"] != "hello bluesky" {
			t.Errorf("expected text 'hello bluesky', got %v", record["text"])
		}
		if body["repo"] != "did:plc:123" {
			t.Errorf("expected repo did:plc:123, got %v", body["repo"])
		}

		// Respond with success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(blueskyCreateRecordResponse{
			URI: "at://did:plc:123/app.bsky.feed.post/456",
			CID: "bafyre...",
		})
	}))
	defer server.Close()

	p := NewBlueskyProvider()
	account := domain.SocialAccount{
		InstanceURL:     server.URL,
		RemoteAccountID: "did:plc:123",
		Username:        "user.bsky.social",
		AuthType:        domain.AccountAuthTypeOAuthToken,
	}
	auth := PublishAuth{AccessToken: "test-token"}
	req := PublishRequest{Content: "hello bluesky"}

	result, err := p.Publish(context.Background(), account, auth, req)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if result.RemoteID != "at://did:plc:123/app.bsky.feed.post/456" {
		t.Errorf("expected RemoteID at://did:plc:123/app.bsky.feed.post/456, got %s", result.RemoteID)
	}
	expectedURL := "https://bsky.app/profile/user.bsky.social/post/456"
	if result.URL != expectedURL {
		t.Errorf("expected URL %s, got %s", expectedURL, result.URL)
	}
}

func TestMastodonProvider_ConnectAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/accounts/verify_credentials" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mastodonAccountResponse{
				ID:       "acc123",
				Username: "testuser",
				Acct:     "testuser@example.com",
			})
			return
		}
		t.Errorf("unexpected path %s", r.URL.Path)
	}))
	defer server.Close()

	p := NewMastodonProvider(MastodonRegistrationConfig{})
	input := domain.CreateAccountInput{
		InstanceURL: server.URL,
		AccessToken: "valid-token",
	}

	ctx := WithOutboundInstancePolicy(context.Background(), OutboundPolicy{AllowPrivateLAN: true})
	account, err := p.ConnectAccount(ctx, input, nil)
	if err != nil {
		t.Fatalf("ConnectAccount failed: %v", err)
	}

	if account.Username != "testuser@example.com" {
		t.Errorf("expected username testuser@example.com, got %s", account.Username)
	}
	if account.RemoteAccountID != "acc123" {
		t.Errorf("expected RemoteAccountID acc123, got %s", account.RemoteAccountID)
	}
}

func TestBlueskyProvider_ConnectAccount_AppPassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/xrpc/com.atproto.server.createSession" {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["identifier"] != "user.bsky.social" || body["password"] != "app-pass" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(blueskySessionResponse{
				DID:    "did:plc:123",
				Handle: "user.bsky.social",
			})
			return
		}
		t.Errorf("unexpected path %s", r.URL.Path)
	}))
	defer server.Close()

	p := NewBlueskyProvider()
	input := domain.CreateAccountInput{
		InstanceURL: server.URL,
		Identifier:  "user.bsky.social",
		AppPassword: "app-pass",
	}

	ctx := WithOutboundInstancePolicy(context.Background(), OutboundPolicy{AllowPrivateLAN: true})
	account, err := p.ConnectAccount(ctx, input, nil)
	if err != nil {
		t.Fatalf("ConnectAccount failed: %v", err)
	}

	if account.AuthType != domain.AccountAuthTypeAppPassword {
		t.Errorf("expected AuthType app_password, got %s", account.AuthType)
	}
	if account.Username != "user.bsky.social" {
		t.Errorf("expected username user.bsky.social, got %s", account.Username)
	}
}

func TestMastodonProvider_Publish_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
	}))
	defer server.Close()

	p := NewMastodonProvider(MastodonRegistrationConfig{})
	account := domain.SocialAccount{InstanceURL: server.URL}
	_, err := p.Publish(context.Background(), account, PublishAuth{AccessToken: "bad"}, PublishRequest{Content: "hi"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected error to contain 'status 401', got: %v", err)
	}
}

func TestMastodonProvider_GetMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("expected Bearer tok, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/statuses/999":
			json.NewEncoder(w).Encode(map[string]any{
				"favourites_count": 7,
				"reblogs_count":    2,
				"replies_count":    1,
			})
		case "/api/v1/statuses/999/context":
			json.NewEncoder(w).Encode(map[string]any{
				"descendants": []any{
					map[string]any{"id": "1"},
					map[string]any{"id": "2"},
					map[string]any{"id": "3"},
					map[string]any{"id": "4"},
				},
			})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p := NewMastodonProvider(MastodonRegistrationConfig{})
	account := domain.SocialAccount{InstanceURL: server.URL}
	out, err := p.GetMetrics(context.Background(), account, PublishAuth{AccessToken: "tok"}, "https://social.example/@x/999")
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	m := map[string]int64{}
	for _, x := range out {
		m[x.Name] = x.Value
	}
	if m["likes"] != 7 || m["reposts"] != 2 || m["replies"] != 4 {
		t.Fatalf("metrics: %#v", m)
	}
}

func TestBlueskyProvider_GetMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xrpc/app.bsky.feed.getPosts" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		want := "at://did:plc:123/app.bsky.feed.post/abc"
		if r.URL.Query().Get("uris") != want {
			t.Errorf("uris=%q want %q", r.URL.Query().Get("uris"), want)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"posts": []any{
				map[string]any{
					"post": map[string]any{
						"likeCount":   3,
						"repostCount": 4,
						"replyCount":  5,
						"quoteCount":  6,
					},
				},
			},
		})
	}))
	defer server.Close()

	p := NewBlueskyProvider()
	account := domain.SocialAccount{
		InstanceURL:     server.URL,
		RemoteAccountID: "did:plc:123",
		Username:        "user.bsky.social",
		AuthType:        domain.AccountAuthTypeOAuthToken,
	}
	out, err := p.GetMetrics(context.Background(), account, PublishAuth{AccessToken: "jwt"}, "https://bsky.app/profile/user.bsky.social/post/abc")
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	m := map[string]int64{}
	for _, x := range out {
		m[x.Name] = x.Value
	}
	if m["likes"] != 3 || m["reposts"] != 4 || m["replies"] != 5 || m["quotes"] != 6 {
		t.Fatalf("metrics: %#v", m)
	}
}

func TestFriendicaProvider_GetMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fc" {
			t.Errorf("expected Bearer fc, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/statuses/42":
			json.NewEncoder(w).Encode(map[string]any{
				"favourites_count": 1,
				"reblogs_count":    2,
				"replies_count":    3,
			})
		case "/api/v1/statuses/42/context":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p := NewFriendicaProvider()
	account := domain.SocialAccount{InstanceURL: server.URL}
	out, err := p.GetMetrics(context.Background(), account, PublishAuth{AccessToken: "fc"}, "https://friendica.example/statuses/42")
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	m := map[string]int64{}
	for _, x := range out {
		m[x.Name] = x.Value
	}
	if m["likes"] != 1 || m["reposts"] != 2 || m["replies"] != 3 {
		t.Fatalf("metrics: %#v", m)
	}
}

func TestMastodonProvider_GetMetrics_requiresPublishedURL(t *testing.T) {
	p := NewMastodonProvider(MastodonRegistrationConfig{})
	_, err := p.GetMetrics(context.Background(), domain.SocialAccount{InstanceURL: "https://x"}, PublishAuth{AccessToken: "t"}, "")
	if err == nil {
		t.Fatal("expected error for empty published URL")
	}
}

func TestMastodonProvider_UploadMedia(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/media" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("auth header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "99", "url": "http://cdn/ready"})
	}))
	defer server.Close()

	p := NewMastodonProvider(MastodonRegistrationConfig{})
	acc := domain.SocialAccount{InstanceURL: server.URL}
	id, err := p.UploadMedia(context.Background(), acc, PublishAuth{AccessToken: "tok"}, strings.NewReader("x"), "a.png", "image/png", "alt")
	if err != nil {
		t.Fatal(err)
	}
	if id != "99" {
		t.Fatalf("media id: %q", id)
	}
}

func TestMastodonProvider_RefreshAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/token" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	mp := NewMastodonProvider(MastodonRegistrationConfig{}).(*MastodonProvider)
	inst := domain.ProviderInstance{
		InstanceURL:   server.URL,
		ClientID:      "cid",
		TokenEndpoint: server.URL + "/oauth/token",
	}
	access, refresh, exp, err := mp.RefreshAccessToken(context.Background(), inst, "secret", "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if access != "new-access" || refresh != "new-refresh" {
		t.Fatalf("tokens %q %q", access, refresh)
	}
	if exp == nil || time.Until(*exp) < time.Hour-time.Minute || time.Until(*exp) > time.Hour+time.Minute {
		t.Fatalf("unexpected expiry: %v", exp)
	}
}

func TestBlueskyProvider_GetMetrics_invalidPublishedURL(t *testing.T) {
	p := NewBlueskyProvider()
	account := domain.SocialAccount{
		InstanceURL:     "https://bsky.example",
		RemoteAccountID: "did:plc:1",
		Username:        "u.bsky.social",
		AuthType:        domain.AccountAuthTypeOAuthToken,
	}
	_, err := p.GetMetrics(context.Background(), account, PublishAuth{AccessToken: "jwt"}, "https://example.com/not-a-post")
	if err == nil {
		t.Fatal("expected error for URL without post rkey")
	}
}
