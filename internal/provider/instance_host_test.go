package provider

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestNormalizeInstanceURL_blocksLoopbackWithoutPolicy(t *testing.T) {
	t.Parallel()
	_, err := normalizeInstanceURL(context.Background(), "http://127.0.0.1:8080")
	if err == nil {
		t.Fatal("expected error for loopback URL")
	}
	if !strings.Contains(err.Error(), "non-public") && !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeInstanceURL_allowsLoopbackWithPolicy(t *testing.T) {
	t.Parallel()
	ctx := WithOutboundInstancePolicy(context.Background(), OutboundPolicy{AllowPrivateLAN: true})
	got, err := normalizeInstanceURL(ctx, "http://127.0.0.1:8080/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://127.0.0.1:8080/path" {
		t.Fatalf("want trimmed URL without trailing slash on path segment - got %q", got)
	}
}

func TestNormalizeInstanceURL_blocksRFC1918(t *testing.T) {
	t.Parallel()
	_, err := normalizeInstanceURL(context.Background(), "https://10.0.0.1/")
	if err == nil {
		t.Fatal("expected error for RFC1918 address")
	}
}

func TestIsForbiddenHostname(t *testing.T) {
	t.Parallel()
	cases := []struct {
		host    string
		blocked bool
	}{
		{"localhost", true},
		{"LOCALHOST", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"0.0.0.0", true},
		{"mastodon.social", false},
		{"social.example.org", false},
		{"", false},
		{"  localhost  ", true}, // trimmed
	}
	for _, tc := range cases {
		got := isForbiddenHostname(tc.host)
		if got != tc.blocked {
			t.Errorf("isForbiddenHostname(%q) = %v, want %v", tc.host, got, tc.blocked)
		}
	}
}

func TestIsNonPublicIP(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ip       string
		nonPublic bool
	}{
		// Loopback
		{"127.0.0.1", true},
		{"::1", true},
		// Private RFC1918
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.100", true},
		// Link-local
		{"169.254.1.1", true},
		{"fe80::1", true},
		// Unspecified
		{"0.0.0.0", true},
		// Public
		{"1.1.1.1", false},
		{"8.8.8.8", false},
		{"2606:4700:4700::1111", false},
	}
	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("invalid IP address in test case: %q", tc.ip)
		}
		got := isNonPublicIP(ip)
		if got != tc.nonPublic {
			t.Errorf("isNonPublicIP(%q) = %v, want %v", tc.ip, got, tc.nonPublic)
		}
	}
}

func TestIsNonPublicIP_Nil(t *testing.T) {
	if !isNonPublicIP(nil) {
		t.Error("isNonPublicIP(nil) should return true")
	}
}
