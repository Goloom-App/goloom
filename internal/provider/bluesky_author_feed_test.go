package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestBlueskyListAuthorPosts_skipsRepostsAndReplies(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/xrpc/app.bsky.feed.getAuthorFeed") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"feed": [
				{
					"post": {
						"uri": "at://did:plc:x/app.bsky.feed.post/original",
						"record": {"text": "Original", "createdAt": "2026-06-01T12:00:00.000Z"}
					}
				},
				{
					"reason": {"$type": "app.bsky.feed.defs#reasonRepost"},
					"post": {
						"uri": "at://did:plc:x/app.bsky.feed.post/repost",
						"record": {"text": "Repost", "createdAt": "2026-06-01T12:00:00.000Z"}
					}
				},
				{
					"post": {
						"uri": "at://did:plc:x/app.bsky.feed.post/reply",
						"record": {
							"text": "Reply",
							"createdAt": "2026-06-01T12:00:00.000Z",
							"reply": {"root": {"uri": "at://did:plc:x/app.bsky.feed.post/root"}, "parent": {"uri": "at://did:plc:x/app.bsky.feed.post/parent"}}
						}
					}
				}
			]
		}`))
	}))
	defer srv.Close()

	p := NewBlueskyProvider().(AuthorFeedFetcher)
	account := domain.SocialAccount{
		InstanceURL:     srv.URL,
		RemoteAccountID: "did:plc:x",
		Username:        "user.bsky.social",
		AuthType:        domain.AccountAuthTypeOAuthToken,
	}
	posts, err := p.ListAuthorPosts(context.Background(), account, PublishAuth{AccessToken: "jwt"}, time.Time{}, 10)
	if err != nil {
		t.Fatalf("ListAuthorPosts: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("want 1 original post, got %d", len(posts))
	}
	if posts[0].RemoteID != "at://did:plc:x/app.bsky.feed.post/original" {
		t.Fatalf("unexpected remote id %q", posts[0].RemoteID)
	}
}
