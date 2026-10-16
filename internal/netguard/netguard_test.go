package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

type fakeResolver struct {
	addrs map[string][]netip.Addr
}

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f.addrs[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

func testPolicy(allowed ...string) Policy {
	p := NewPolicy(allowed)
	p.resolver = fakeResolver{addrs: map[string][]netip.Addr{
		"api.example":    {netip.MustParseAddr("93.184.216.34")},
		"internal.corp":  {netip.MustParseAddr("10.0.0.7")},
		"rebind.example": {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("169.254.169.254")},
		"ollama.lan":     {netip.MustParseAddr("192.168.1.20")},
	}}
	return p
}

func TestIsPublicAddr(t *testing.T) {
	cases := []struct {
		addr   string
		public bool
	}{
		{"8.8.8.8", true},
		{"2606:4700:4700::1112", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false},
		{"100.64.0.1", false},
		{"0.0.0.0", false},
		{"::1", false},
		{"fd00::1", false},
		{"fe80::1", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:8.8.8.8", true},
		// IPv6 forms embedding an IPv4 address are judged by that address.
		{"64:ff9b::7f00:1", false},    // NAT64 → 127.0.0.1
		{"64:ff9b::a9fe:a9fe", false}, // NAT64 → 169.254.169.254
		{"::7f00:1", false},           // IPv4-compatible (deprecated)
		{"::ffff:0:7f00:1", false},    // SIIT
		{"64:ff9b::808:808", true},    // NAT64 → 8.8.8.8
	}
	for _, tc := range cases {
		if got := IsPublicAddr(netip.MustParseAddr(tc.addr)); got != tc.public {
			t.Errorf("IsPublicAddr(%s) = %v, want %v", tc.addr, got, tc.public)
		}
	}
}

func TestCheckURL(t *testing.T) {
	cases := []struct {
		name    string
		policy  Policy
		raw     string
		wantErr bool
	}{
		{"public https host", testPolicy(), "https://api.example/v1", false},
		{"public ip literal", testPolicy(), "https://8.8.8.8/v1", false},
		{"plain http public", testPolicy(), "http://api.example", false},
		{"unsupported scheme", testPolicy(), "ftp://api.example", true},
		{"no scheme", testPolicy(), "api.example/v1", true},
		{"credentials in url", testPolicy(), "https://user:pass@api.example", true},
		{"missing host", testPolicy(), "https:///v1", true},
		{"loopback literal", testPolicy(), "http://127.0.0.1:11434/v1", true},
		{"ipv6 loopback", testPolicy(), "http://[::1]:8080", true},
		{"localhost name", testPolicy(), "http://localhost:11434", true},
		{"metadata endpoint", testPolicy(), "http://169.254.169.254/latest", true},
		{"private resolved host", testPolicy(), "https://internal.corp", true},
		{"mixed resolution", testPolicy(), "https://rebind.example", true},
		{"unresolvable host", testPolicy(), "https://nowhere.example", true},
		{"allowlisted private host", testPolicy("ollama.lan"), "http://ollama.lan:11434/v1", false},
		{"allowlist is case-insensitive", testPolicy("Ollama.LAN"), "http://OLLAMA.lan:11434", false},
		{"allowlisted localhost", testPolicy("localhost"), "http://localhost:11434", false},
		{"allowlist does not cover other hosts", testPolicy("ollama.lan"), "https://internal.corp", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.CheckURL(context.Background(), tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("CheckURL(%q) err=%v, wantErr=%v", tc.raw, err, tc.wantErr)
			}
		})
	}
}

func TestDialContextRefusesNonPublic(t *testing.T) {
	dialed := false
	dial := testPolicy().DialContext(func(_ context.Context, _, _ string) (net.Conn, error) {
		dialed = true
		return nil, nil
	})
	for _, addr := range []string{"internal.corp:443", "rebind.example:443", "127.0.0.1:80", "[::1]:80", "nowhere.example:443"} {
		if _, err := dial(context.Background(), "tcp", addr); err == nil {
			t.Errorf("%s: expected refusal", addr)
		}
	}
	if dialed {
		t.Fatal("base dialer must not run for non-public targets")
	}
}

func TestDialContextDialsResolvedPublicIP(t *testing.T) {
	var got string
	dial := testPolicy().DialContext(func(_ context.Context, _, addr string) (net.Conn, error) {
		got = addr
		return nil, errors.New("stop")
	})
	_, _ = dial(context.Background(), "tcp", "api.example:443")
	if got != "93.184.216.34:443" {
		t.Fatalf("dialed %q, want resolved public IP", got)
	}
}

func TestDialContextPassesAllowlistedHost(t *testing.T) {
	var got string
	dial := testPolicy("ollama.lan").DialContext(func(_ context.Context, _, addr string) (net.Conn, error) {
		got = addr
		return nil, errors.New("stop")
	})
	_, _ = dial(context.Background(), "tcp", "ollama.lan:11434")
	if got != "ollama.lan:11434" {
		t.Fatalf("dialed %q, want allowlisted host passed through", got)
	}
}

func TestHTTPClientBlocksLoopbackServer(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer srv.Close()

	resp, err := NewPolicy(nil).HTTPClient(5 * time.Second).Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected guarded client to refuse a loopback server")
	}
	if hit {
		t.Fatal("loopback server must not receive the request")
	}
}

func TestHTTPClientAllowsAllowlistedLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer srv.Close()

	resp, err := NewPolicy([]string{"127.0.0.1"}).HTTPClient(5 * time.Second).Get(srv.URL)
	if err != nil {
		t.Fatalf("allowlisted loopback refused: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestHTTPClientRefusesRedirectToNonPublic(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("redirect target must not be reached")
	}))
	defer internal.Close()
	// The entry server is allowlisted (stands in for a public host); the
	// redirect target is a different, non-allowlisted loopback name.
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost:"+internal.URL[len("http://127.0.0.1:"):], http.StatusFound)
	}))
	defer entry.Close()

	resp, err := NewPolicy([]string{"127.0.0.1"}).HTTPClient(5 * time.Second).Get(entry.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected redirect to non-allowlisted loopback host to fail")
	}
}

func TestAllowlistEntries(t *testing.T) {
	p := NewPolicy([]string{" Ollama.lan ", "", "localhost:11434", "[::1]:8080"})
	cases := []struct {
		host, port string
		want       bool
	}{
		{"ollama.lan", "11434", true},
		{"OLLAMA.lan", "80", true}, // entry without port allows any port
		{"localhost", "11434", true},
		{"localhost", "5432", false}, // entry with port allows only that port
		{"::1", "8080", true},
		{"::1", "22", false},
		{"", "80", false},
	}
	for _, tc := range cases {
		if got := p.allows(tc.host, tc.port); got != tc.want {
			t.Errorf("allows(%q, %q) = %v, want %v", tc.host, tc.port, got, tc.want)
		}
	}
}

func TestCheckURLAllowlistPort(t *testing.T) {
	p := testPolicy("localhost:11434")
	if err := p.CheckURL(context.Background(), "http://localhost:11434/v1"); err != nil {
		t.Fatalf("allowlisted host:port refused: %v", err)
	}
	if err := p.CheckURL(context.Background(), "http://localhost:5432/"); err == nil {
		t.Fatal("other port on allowlisted host must be refused")
	}
	if err := testPolicy("api.example:8443").CheckURL(context.Background(), "https://api.example/v1"); err != nil {
		t.Fatalf("public host must pass regardless of allowlist: %v", err)
	}
}
