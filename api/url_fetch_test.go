package api

import (
	"context"
	"git.f4mily.net/goloom/internal/netguard"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A non-2xx response must surface the HTTP status code, so a failed fetch is
// diagnosable instead of an opaque "url fetch failed" (the niri link the agent
// could not load gave no clue why).
func TestFetchURLBody_SurfacesStatusCode(t *testing.T) {
	allowLoopbackFetch(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer server.Close()

	_, err := fetchURLBody(context.Background(), server.URL)
	if err == nil {
		t.Fatal("a 403 response must produce an error")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error must name the HTTP status, got %q", err)
	}
}

// fetch_url targets are model-controlled (and thus injection-controllable), so
// fetches must never reach loopback, private or link-local addresses.
func TestFetchURLBody_BlocksPrivateTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("internal secret"))
	}))
	defer server.Close()

	for _, target := range []string{server.URL, "http://169.254.169.254/latest/meta-data/"} {
		body, err := fetchURLBody(context.Background(), target)
		if err == nil {
			t.Fatalf("fetch of %s must be blocked, got body %q", target, body)
		}
	}
}

// An allowlisted (or public) host that redirects to a non-public address must
// not be followed: the redirect target is re-checked against the policy.
func TestFetchURLBody_RefusesRedirectToPrivateAddress(t *testing.T) {
	var hit atomic.Bool
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Store(true)
		_, _ = w.Write([]byte("internal secret"))
	}))
	defer internal.Close()
	// 127.0.0.2 reaches the same listener but is not on the allowlist.
	internalURL := strings.Replace(internal.URL, "127.0.0.1", "127.0.0.2", 1)

	for _, target := range []string{internalURL, "http://169.254.169.254/latest/meta-data/"} {
		redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target, http.StatusFound)
		}))
		allowLoopbackFetch(t)
		body, err := fetchURLBody(context.Background(), redirector.URL)
		redirector.Close()
		if err == nil {
			t.Fatalf("redirect to %s must be refused, got body %q", target, body)
		}
		assertGuardRefusal(t, target, err)
	}
	if hit.Load() {
		t.Fatal("redirect target was contacted despite the guard")
	}
}

func TestFetchURLBody_RefusesDisallowedSchemesAndHosts(t *testing.T) {
	for _, target := range []string{
		"file:///etc/passwd",
		"ftp://example.com/x",
		"gopher://example.com/",
	} {
		_, err := fetchURLBody(context.Background(), target)
		if err == nil || !strings.Contains(err.Error(), "url must use http or https") {
			t.Errorf("fetch of %q must fail URL validation, got %v", target, err)
		}
	}
	if _, err := fetchURLBody(context.Background(), "http://user:pass@127.0.0.1/"); err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("credential-bearing URL must fail validation, got %v", err)
	}
	for _, target := range []string{
		"http://localhost/",
		"http://127.0.0.1:1/",
		"http://[::1]/",
		"http://10.0.0.1/",
		"http://192.168.1.1/",
		"http://169.254.169.254/",
	} {
		body, err := fetchURLBody(context.Background(), target)
		if err == nil {
			t.Errorf("fetch of %q must be refused, got body %q", target, body)
			continue
		}
		assertGuardRefusal(t, target, err)
	}
}

// Environment proxies would hide the real target from the dial check, so the
// fetch client must ignore them. (Go skips proxies for loopback destinations,
// so a request-level test against httptest cannot detect this; assert on the
// transport that httpGetBody uses instead.)
func TestURLFetchClient_IgnoresEnvProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("http_proxy", "http://127.0.0.1:9")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("https_proxy", "http://127.0.0.1:9")

	client := urlFetchPolicy.HTTPClient(time.Second)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("fetch client must not use a proxy from the environment")
	}
}

// assertGuardRefusal fails unless err comes from the netguard policy, so a
// plain connection error (which would also "refuse" the fetch) cannot mask a
// missing guard. The guard rejects before any connection is dialed.
func assertGuardRefusal(t *testing.T, target string, err error) {
	t.Helper()
	msg := err.Error()
	if !strings.Contains(msg, "non-public") && !strings.Contains(msg, "not allowed") {
		t.Errorf("%s: error must come from the netguard policy, got %q", target, msg)
	}
}

// allowLoopbackFetch lets a test fetch from its own httptest server.
func allowLoopbackFetch(t *testing.T) {
	t.Helper()
	prev := urlFetchPolicy
	urlFetchPolicy = netguard.NewPolicy([]string{"127.0.0.1"})
	t.Cleanup(func() { urlFetchPolicy = prev })
}
