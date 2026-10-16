package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func loopbackLLM(t *testing.T, hit *bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*hit = true
		io.WriteString(w, `{"model":"gpt-4o","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"hi"}}]}`)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestDefaultClientRefusesNonPublicBaseURL(t *testing.T) {
	var hit bool
	server := loopbackLLM(t, &hit)

	client, err := NewClient(Settings{Provider: ProviderOpenAI, Model: "gpt-4o", APIKey: "sk-secret", BaseURL: server.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err == nil {
		t.Fatal("expected default client to refuse a loopback base URL")
	}
	if hit {
		t.Fatal("API key must not reach a non-public host")
	}
}

func TestServiceRefusesNonPublicBaseURL(t *testing.T) {
	var hit bool
	server := loopbackLLM(t, &hit)

	client, err := NewService().ClientFor(domainConfig(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err == nil {
		t.Fatal("expected service client to refuse a loopback base URL")
	}
	if hit {
		t.Fatal("API key must not reach a non-public host")
	}
}

func TestServiceAllowsAllowlistedPrivateHost(t *testing.T) {
	var hit bool
	server := loopbackLLM(t, &hit)

	client, err := NewService("127.0.0.1").ClientFor(domainConfig(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
		t.Fatalf("allowlisted host refused: %v", err)
	}
	if !hit {
		t.Fatal("allowlisted host was not called")
	}
}

func domainConfig(baseURL string) domain.AIServiceConfig {
	return domain.AIServiceConfig{Provider: ProviderOpenAI, Model: "gpt-4o", APIKey: "sk-secret", BaseURL: baseURL}
}

func TestHTTPClientDoesNotFollowRedirects(t *testing.T) {
	var targetHit bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetHit = true }))
	t.Cleanup(target.Close)
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(entry.Close)

	// Both servers are allowlisted: the API key header must still never be
	// forwarded to a redirect target.
	req, _ := http.NewRequest(http.MethodPost, entry.URL, nil)
	req.Header.Set("x-api-key", "sk-secret")
	resp, err := NewHTTPClient([]string{"127.0.0.1"}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want the unfollowed 307", resp.StatusCode)
	}
	if targetHit {
		t.Fatal("redirect target must not be called")
	}
}
