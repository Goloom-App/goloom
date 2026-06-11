package htmltext

import (
	"strings"
	"testing"
)

func TestExtractReadableText_stripsScriptsAndKeepsArticle(t *testing.T) {
	html := `<html><head><title>x</title><script>alert(1)</script></head>
<body><article><h1>Episode 42</h1><p>We talk about <strong>WireGuard</strong> and self-hosting.</p></article></body></html>`
	text := ExtractReadableText(html)
	if text == "" {
		t.Fatal("expected non-empty text")
	}
	for _, want := range []string{"Episode 42", "WireGuard", "self-hosting"} {
		if !strings.Contains(text, want) {
			t.Fatalf("ExtractReadableText = %q, missing %q", text, want)
		}
	}
	for _, bad := range []string{"<script>", "alert"} {
		if strings.Contains(text, bad) {
			t.Fatalf("script leaked into text: %q", text)
		}
	}
}

func TestExtractReadableText_plainTextPassthrough(t *testing.T) {
	raw := "Plain show notes without markup."
	if got := ExtractReadableText(raw); got != raw {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestExtractReadableText_truncatesVeryLongInput(t *testing.T) {
	long := strings.Repeat("a", maxExtractLen+10)
	got := ExtractReadableText(long)
	if len([]rune(got)) != maxExtractLen+1 {
		t.Fatalf("expected truncated length %d, got %d", maxExtractLen+1, len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
}
