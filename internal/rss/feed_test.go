package rss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- ItemKey ---

func TestItemKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		item Item
		want string
	}{
		{
			name: "GUID takes priority",
			item: Item{GUID: "urn:uuid:1234", Link: "https://example.com/1"},
			want: "urn:uuid:1234",
		},
		{
			name: "falls back to Link when GUID empty",
			item: Item{GUID: "", Link: "https://example.com/1"},
			want: "https://example.com/1",
		},
		{
			name: "GUID with whitespace is trimmed",
			item: Item{GUID: "  g1  ", Link: "https://example.com"},
			want: "g1",
		},
		{
			name: "whitespace-only GUID falls back to Link",
			item: Item{GUID: "   ", Link: "https://example.com/2"},
			want: "https://example.com/2",
		},
		{
			name: "both empty returns empty string",
			item: Item{GUID: "", Link: ""},
			want: "",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ItemKey(tc.item); got != tc.want {
				t.Errorf("ItemKey(%+v) = %q, want %q", tc.item, got, tc.want)
			}
		})
	}
}

// --- mapItem ---

const rssFeedWithGUID = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <title>First Post</title>
      <link>https://example.com/1</link>
      <guid>urn:uuid:first</guid>
      <description>Short desc</description>
      <content:encoded xmlns:content="http://purl.org/rss/1.0/modules/content/">Full content</content:encoded>
      <pubDate>Mon, 01 Jan 2024 10:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

const rssFeedWithoutGUID = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <title>  Second Post  </title>
      <link>  https://example.com/2  </link>
      <description>Only description</description>
    </item>
  </channel>
</rss>`

const atomFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Feed</title>
  <entry>
    <id>tag:example.com,2024:1</id>
    <title>Atom Entry</title>
    <link href="https://example.com/atom/1"/>
    <updated>2024-03-15T09:00:00Z</updated>
    <summary>Atom summary</summary>
  </entry>
</feed>`

const emptyFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Empty Feed</title>
  </channel>
</rss>`

// --- NewParser ---

func TestNewParser_NotNil(t *testing.T) {
	p := NewParser()
	if p == nil {
		t.Fatal("NewParser returned nil")
	}
	if p.client == nil {
		t.Fatal("NewParser: inner client is nil")
	}
}

// --- Fetch ---

func TestFetch_RSSWithGUID(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssFeedWithGUID))
	}))
	defer srv.Close()

	p := NewParser()
	items, err := p.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item.GUID != "urn:uuid:first" {
		t.Errorf("GUID = %q", item.GUID)
	}
	if item.Link != "https://example.com/1" {
		t.Errorf("Link = %q", item.Link)
	}
	if item.Title != "First Post" {
		t.Errorf("Title = %q", item.Title)
	}
	// Content:encoded should take priority over description.
	if item.Content != "Full content" {
		t.Errorf("Content = %q, want 'Full content'", item.Content)
	}
}

func TestFetch_RSSWithoutGUID_FallsBackToLink(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssFeedWithoutGUID))
	}))
	defer srv.Close()

	p := NewParser()
	items, err := p.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	// No GUID in source → GUID should be set to the link.
	if item.GUID != "https://example.com/2" {
		t.Errorf("GUID = %q, want link as fallback", item.GUID)
	}
	if item.Title != "Second Post" {
		t.Errorf("Title = %q (should be trimmed)", item.Title)
	}
	if item.Content != "Only description" {
		t.Errorf("Content = %q", item.Content)
	}
}

func TestFetch_AtomFeed(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(atomFeed))
	}))
	defer srv.Close()

	p := NewParser()
	items, err := p.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.Title != "Atom Entry" {
		t.Errorf("Title = %q", item.Title)
	}
	if item.Link != "https://example.com/atom/1" {
		t.Errorf("Link = %q", item.Link)
	}
	// Updated date used as publication date fallback.
	if item.PublishedAt.IsZero() {
		t.Error("PublishedAt should not be zero for Atom entry with updated date")
	}
}

func TestFetch_EmptyFeed(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(emptyFeed))
	}))
	defer srv.Close()

	p := NewParser()
	items, err := p.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items from empty feed, got %d", len(items))
	}
}

func TestFetch_Non200Response(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewParser()
	_, err := p.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestFetch_InvalidXML(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<not valid xml at all <<< >>>"))
	}))
	defer srv.Close()

	p := NewParser()
	_, err := p.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for malformed XML")
	}
}

func TestFetch_EmptyURL(t *testing.T) {
	t.Parallel()
	p := NewParser()
	_, err := p.Fetch(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestFetch_WhitespaceURL(t *testing.T) {
	t.Parallel()
	p := NewParser()
	_, err := p.Fetch(context.Background(), "   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only URL")
	}
}

func TestFetch_CancelledContext(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond — wait for client to give up.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	p := NewParser()
	_, err := p.Fetch(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// --- mapItem edge cases via Fetch ---

func TestFetch_PublishedAtFallsBackToUpdated(t *testing.T) {
	t.Parallel()
	// Atom entry with no published but with updated — should use updated.
	atomWithUpdatedOnly := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Feed</title>
  <entry>
    <id>1</id>
    <title>Entry</title>
    <link href="https://example.com/1"/>
    <updated>2024-06-01T12:00:00Z</updated>
  </entry>
</feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(atomWithUpdatedOnly))
	}))
	defer srv.Close()

	p := NewParser()
	items, err := p.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	want := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	if !items[0].PublishedAt.Equal(want) {
		t.Errorf("PublishedAt = %v, want %v", items[0].PublishedAt, want)
	}
}

func TestFetch_ContentPreferredOverDescription(t *testing.T) {
	t.Parallel()
	// RSS where content:encoded exists alongside description.
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Feed</title>
    <item>
      <title>Post</title>
      <link>https://example.com/1</link>
      <guid>g1</guid>
      <description>Short</description>
      <content:encoded>Long full content</content:encoded>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(feed))
	}))
	defer srv.Close()

	p := NewParser()
	items, err := p.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	if items[0].Content != "Long full content" {
		t.Errorf("Content = %q, want content:encoded to be preferred", items[0].Content)
	}
}

func TestFetch_DescriptionFallbackWhenNoContent(t *testing.T) {
	t.Parallel()
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Feed</title>
    <item>
      <title>Post</title>
      <link>https://example.com/1</link>
      <description>Only description available</description>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(feed))
	}))
	defer srv.Close()

	p := NewParser()
	items, err := p.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	if !strings.Contains(items[0].Content, "Only description available") {
		t.Errorf("Content = %q, want description as fallback", items[0].Content)
	}
}

func TestFetch_TitleAndLinkTrimmed(t *testing.T) {
	t.Parallel()
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Feed</title>
    <item>
      <title>  spaced title  </title>
      <link>  https://example.com/spaced  </link>
      <description>body</description>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(feed))
	}))
	defer srv.Close()

	p := NewParser()
	items, err := p.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	if items[0].Title != "spaced title" {
		t.Errorf("Title = %q (not trimmed)", items[0].Title)
	}
	if items[0].Link != "https://example.com/spaced" {
		t.Errorf("Link = %q (not trimmed)", items[0].Link)
	}
}
