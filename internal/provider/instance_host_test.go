package provider

import (
	"context"
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
