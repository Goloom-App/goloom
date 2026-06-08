package rss

import (
	"testing"
	"time"
)

func TestExpandContent(t *testing.T) {
	ts := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	got := ExpandContent("{title} — {link} (#{counter})", ItemFields{
		Title:       "Hello",
		Link:        "https://example.com/a",
		Summary:     "<p>Body</p>",
		FeedName:    "Blog",
		PublishedAt: ts,
		Counter:     3,
	})
	want := "Hello — https://example.com/a (#3)"
	if got != want {
		t.Fatalf("ExpandContent = %q, want %q", got, want)
	}
}

func TestStripHTML(t *testing.T) {
	got := StripHTML("<p>Hello <b>world</b></p>")
	if got != "Hello world" {
		t.Fatalf("StripHTML = %q", got)
	}
}
