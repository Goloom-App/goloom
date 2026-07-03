package provider

import (
	"testing"
)

func TestResolveRemotePostID(t *testing.T) {
	t.Run("BlueskyPrefersURIFromMetadata", func(t *testing.T) {
		result := PublishResult{
			RemoteID: "remote123",
			Metadata: map[string]string{"uri": "at://did:plc:abc/app.bsky.feed.post/xyz"},
		}
		got := ResolveRemotePostID("bluesky", result)
		want := "at://did:plc:abc/app.bsky.feed.post/xyz"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("BlueskyFallsBackToRemoteIDWhenNoURI", func(t *testing.T) {
		result := PublishResult{RemoteID: "remote456", Metadata: map[string]string{}}
		got := ResolveRemotePostID("bluesky", result)
		if got != "remote456" {
			t.Errorf("got %q, want remote456", got)
		}
	})

	t.Run("BlueskyFallsBackToRemoteIDWhenURIEmpty", func(t *testing.T) {
		result := PublishResult{RemoteID: "remote789", Metadata: map[string]string{"uri": "  "}}
		got := ResolveRemotePostID("bluesky", result)
		if got != "remote789" {
			t.Errorf("got %q, want remote789", got)
		}
	})

	t.Run("BlueskyNameCaseInsensitive", func(t *testing.T) {
		result := PublishResult{RemoteID: "r1", Metadata: map[string]string{"uri": "at://uri/1"}}
		got := ResolveRemotePostID("  BLUESKY  ", result)
		if got != "at://uri/1" {
			t.Errorf("uppercase bluesky: got %q", got)
		}
	})

	t.Run("MastodonUsesRemoteID", func(t *testing.T) {
		// mastodon ignores metadata["uri"] — uses RemoteID
		result := PublishResult{RemoteID: "masto123", Metadata: map[string]string{"uri": "https://mastodon.social/some-uri"}}
		got := ResolveRemotePostID("mastodon", result)
		if got != "masto123" {
			t.Errorf("mastodon: got %q, want masto123", got)
		}
	})

	t.Run("FriendicaUsesRemoteID", func(t *testing.T) {
		result := PublishResult{RemoteID: "friendica-99"}
		got := ResolveRemotePostID("friendica", result)
		if got != "friendica-99" {
			t.Errorf("friendica: got %q, want friendica-99", got)
		}
	})

	t.Run("RemoteIDIsWhitespaceTrimmed", func(t *testing.T) {
		result := PublishResult{RemoteID: "  trimmed  "}
		got := ResolveRemotePostID("mastodon", result)
		if got != "trimmed" {
			t.Errorf("trim: got %q, want trimmed", got)
		}
	})

	t.Run("EmptyProvider", func(t *testing.T) {
		result := PublishResult{RemoteID: "r6"}
		got := ResolveRemotePostID("", result)
		if got != "r6" {
			t.Errorf("empty provider: got %q, want r6", got)
		}
	})

	t.Run("NilMetadata", func(t *testing.T) {
		result := PublishResult{RemoteID: "r7", Metadata: nil}
		got := ResolveRemotePostID("bluesky", result)
		// nil metadata map returns empty string for key lookup, so falls back to RemoteID
		if got != "r7" {
			t.Errorf("nil metadata bluesky: got %q, want r7", got)
		}
	})
}
