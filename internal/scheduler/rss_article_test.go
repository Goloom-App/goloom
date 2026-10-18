package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/rss"
)

func TestRSSArticleSourceOnlyFetchesThinOptedInItems(t *testing.T) {
	called := 0
	fetch := func(context.Context, string) (string, error) {
		called++
		return "The linked article gives concrete details about the release.", nil
	}
	item := rss.Item{Link: "https://example.com/article", Content: "Short teaser"}
	for _, feed := range []domain.RSSFeedConfig{{}, {ArticleEnrichmentEnabled: true}} {
		got := rssArticleSource(context.Background(), feed, item, fetch)
		if feed.ArticleEnrichmentEnabled && !strings.Contains(got.Text, "concrete details") {
			t.Fatalf("enriched source = %+v", got)
		}
	}
	if called != 1 {
		t.Fatalf("fetch calls = %d, want 1", called)
	}
	item.Content = strings.Repeat("Full article text. ", 30)
	_ = rssArticleSource(context.Background(), domain.RSSFeedConfig{ArticleEnrichmentEnabled: true}, item, fetch)
	if called != 1 {
		t.Fatal("full RSS content caused article fetch")
	}
}

func TestRSSArticleSourceFallsBackWithNotice(t *testing.T) {
	item := rss.Item{Link: "https://example.com/article", Content: "Feed teaser"}
	got := rssArticleSource(context.Background(), domain.RSSFeedConfig{ArticleEnrichmentEnabled: true}, item, func(context.Context, string) (string, error) {
		return "", errors.New("fetch failed")
	})
	if got.Text != "Feed teaser" || !got.FetchFailed {
		t.Fatalf("fallback source = %+v", got)
	}
}

func TestRSSArticlePreflightSkipsFetchWhenDailyCapReached(t *testing.T) {
	st := &mockStore{countRSSFeedPostsTodayFn: func(context.Context, string) (int, error) { return 1, nil }}
	svc := New(testLogger(), st, nil, 0, 1, 0, 0, 0, 0, nil)
	called := false
	_, _, err := svc.prepareRSSArticle(context.Background(), domain.RSSFeedConfig{ID: "feed", ArticleEnrichmentEnabled: true, MaxPostsPerDay: 1},
		rss.Item{GUID: "item", Link: "https://example.com/article", Content: "thin"}, func(context.Context, string) (string, error) {
			called = true
			return "article", nil
		})
	if !errors.Is(err, domain.ErrRSSDailyLimit) || called {
		t.Fatalf("capped feed fetched article: called=%v err=%v", called, err)
	}
}
