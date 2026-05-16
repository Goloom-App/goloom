package provider

import (
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestMetricsPublishedURL_blueskyPrefersMetadataURI(t *testing.T) {
	account := domain.SocialAccount{Provider: "bluesky"}
	got := MetricsPublishedURL(account, "https://bsky.app/profile/handle/post/abc", map[string]string{
		"uri": "at://did:plc:xyz/app.bsky.feed.post/abc",
	})
	if got != "at://did:plc:xyz/app.bsky.feed.post/abc" {
		t.Fatalf("got %q", got)
	}
}

func TestMetricsPublishedURL_mastodonUsesWebURL(t *testing.T) {
	account := domain.SocialAccount{Provider: "mastodon"}
	got := MetricsPublishedURL(account, "https://social.example/@u/123", map[string]string{
		"uri": "at://ignored",
	})
	if got != "https://social.example/@u/123" {
		t.Fatalf("got %q", got)
	}
}
