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

	account, err := p.ConnectAccount(context.Background(), input, nil)
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

	account, err := p.ConnectAccount(context.Background(), input, nil)
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
