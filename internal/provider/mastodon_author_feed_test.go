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

func TestMastodonListAuthorPosts_excludesRepliesAndReblogs(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/v1/accounts/42/statuses") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("exclude_replies") != "true" || r.URL.Query().Get("exclude_reblogs") != "true" {
			t.Fatalf("expected exclude flags, got %v", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"100","url":"https://m.example/@u/100","uri":"https://m.example/users/u/statuses/100","content":"<p>Hello</p>","created_at":"2026-06-01T12:00:00.000Z"}
		]`))
	}))
	defer srv.Close()

	p := NewMastodonProvider(MastodonRegistrationConfig{}).(AuthorFeedFetcher)
	account := domain.SocialAccount{InstanceURL: srv.URL, RemoteAccountID: "42"}
	posts, err := p.ListAuthorPosts(context.Background(), account, PublishAuth{AccessToken: "tok"}, time.Time{}, 10)
	if err != nil {
		t.Fatalf("ListAuthorPosts: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("want 1 post, got %d", len(posts))
	}
	if posts[0].RemoteID != "100" || posts[0].Content != "Hello" {
		t.Fatalf("unexpected post: %+v", posts[0])
	}
}

func TestStripHTMLContent(t *testing.T) {
	t.Parallel()
	got := stripHTMLContent(`<p>Hi &amp; bye</p>`)
	if got != "Hi & bye" {
		t.Fatalf("got %q", got)
	}
}
