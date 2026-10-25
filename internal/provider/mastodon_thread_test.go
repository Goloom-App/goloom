package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestHTMLToPlainText(t *testing.T) {
	cases := map[string]string{
		"<p>a</p><p>b</p>":               "a\n\nb",
		"<p>x<br>y<br />z</p>":           "x\ny\nz",
		"<p>5 &gt; 3 &amp; more</p>":     "5 > 3 & more",
		"  plain  ":                      "plain",
		`<p><a href="u">@bob</a> hi</p>`: "@bob hi",
	}
	for in, want := range cases {
		if got := htmlToPlainText(in); got != want {
			t.Errorf("htmlToPlainText(%q) = %q, want %q", in, got, want)
		}
	}
}

const threadContextJSON = `{"ancestors":[{"id":"0","content":"ancestor"}],"descendants":[
 {"id":"11","in_reply_to_id":"10","url":"https://m.example/@a/11","uri":"https://m.example/u/a/11","content":"<p>one<br>two</p>","created_at":"2026-01-02T03:04:05.000Z","visibility":"public",
  "account":{"acct":"a@remote.example","display_name":"Alice","avatar_static":"https://m.example/s.png","avatar":"https://m.example/a.gif","url":"https://remote.example/@a"}},
 {"id":"12","in_reply_to_id":"10","content":"<p>secret</p>","created_at":"2026-01-02T03:05:00.000Z","visibility":"direct","account":{"acct":"b"}},
 {"id":"13","in_reply_to_id":"11","uri":"https://m.example/u/c/13","content":"<p>deep</p>","created_at":"2026-01-02T03:06:00.000Z","visibility":"unlisted",
  "account":{"acct":"c","display_name":"Carl","avatar":"https://m.example/c.gif","url":"https://m.example/@c"}}
]}`

func newThreadServer(t *testing.T, wantToken string, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+wantToken {
			t.Errorf("auth header = %q", got)
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMastodonGetThread(t *testing.T) {
	srv := newThreadServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/statuses/10/context" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, threadContextJSON)
	})
	p := NewMastodonProvider(MastodonRegistrationConfig{}).(ThreadFetcher)
	got, err := p.GetThread(context.Background(), domain.SocialAccount{InstanceURL: srv.URL}, PublishAuth{AccessToken: "tok"}, ThreadTarget{PublishedURL: "https://m.example/@me/10"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 replies (direct dropped), got %d: %#v", len(got), got)
	}
	r := got[0]
	if r.RemoteID != "11" || r.ParentID != "10" || r.URL != "https://m.example/@a/11" || r.Content != "one\ntwo" {
		t.Errorf("reply0: %#v", r)
	}
	if r.PublishedAt.IsZero() || r.PublishedAt.Year() != 2026 {
		t.Errorf("published_at: %v", r.PublishedAt)
	}
	want := PostAuthor{Handle: "a@remote.example", DisplayName: "Alice", AvatarURL: "https://m.example/s.png", ProfileURL: "https://remote.example/@a"}
	if r.Author != want {
		t.Errorf("author: %#v", r.Author)
	}
	if got[1].RemoteID != "13" || got[1].ParentID != "11" || got[1].URL != "https://m.example/u/c/13" || got[1].Author.AvatarURL != "https://m.example/c.gif" {
		t.Errorf("reply1: %#v", got[1])
	}
}

func TestFriendicaGetThread_UsesRemoteIDForGUIDURL(t *testing.T) {
	srv := newThreadServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/statuses/42/context" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, threadContextJSON)
	})
	tf, ok := NewFriendicaProvider().(ThreadFetcher)
	if !ok {
		t.Fatal("friendica provider does not implement ThreadFetcher")
	}
	got, err := tf.GetThread(context.Background(), domain.SocialAccount{InstanceURL: srv.URL}, PublishAuth{AccessToken: "tok"},
		ThreadTarget{PublishedURL: "https://f.example/display/abcdef0123456789abcdef0123456789", RemoteID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d", len(got))
	}
}

func TestFriendicaGetThread_GUIDURLWithoutRemoteIDFails(t *testing.T) {
	srv := newThreadServer(t, "tok", func(w http.ResponseWriter, r *http.Request) { t.Error("must not call remote") })
	tf := NewFriendicaProvider().(ThreadFetcher)
	_, err := tf.GetThread(context.Background(), domain.SocialAccount{InstanceURL: srv.URL}, PublishAuth{AccessToken: "tok"},
		ThreadTarget{PublishedURL: "https://f.example/display/abc"})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestMastodonGetThread_RemoteIDWinsOverURL(t *testing.T) {
	srv := newThreadServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/statuses/77/context" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"descendants":[]}`)
	})
	p := NewMastodonProvider(MastodonRegistrationConfig{}).(ThreadFetcher)
	if _, err := p.GetThread(context.Background(), domain.SocialAccount{InstanceURL: srv.URL}, PublishAuth{AccessToken: "tok"},
		ThreadTarget{PublishedURL: "https://m/@a/10", RemoteID: "77"}); err != nil {
		t.Fatal(err)
	}
}

func TestMastodonGetThread_Errors(t *testing.T) {
	p := NewMastodonProvider(MastodonRegistrationConfig{}).(ThreadFetcher)
	acc := func(u string) domain.SocialAccount { return domain.SocialAccount{InstanceURL: u} }
	ctx := context.Background()

	if _, err := p.GetThread(ctx, acc("https://x.example"), PublishAuth{AccessToken: "t"}, ThreadTarget{PublishedURL: ""}); err == nil {
		t.Error("empty url: want error")
	}
	if _, err := p.GetThread(ctx, acc("https://x.example"), PublishAuth{}, ThreadTarget{PublishedURL: "https://x.example/@a/1"}); err == nil {
		t.Error("empty token: want error")
	}

	srv4 := newThreadServer(t, "t", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", 404) })
	srvBad := newThreadServer(t, "t", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "{not json") })
	for name, s := range map[string]*httptest.Server{"404": srv4, "bad json": srvBad} {
		if _, err := p.GetThread(ctx, acc(s.URL), PublishAuth{AccessToken: "t"}, ThreadTarget{PublishedURL: "https://x.example/@a/1"}); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestMastodonGetThread_CapsReplies(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"descendants":[`)
	for i := 1; i <= 250; i++ {
		if i > 1 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"id":"%d","in_reply_to_id":"1","content":"x","created_at":"2026-01-02T03:04:05Z","visibility":"public","account":{"acct":"a"}}`, i+1)
	}
	sb.WriteString(`]}`)
	srv := newThreadServer(t, "t", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, sb.String()) })
	p := NewMastodonProvider(MastodonRegistrationConfig{}).(ThreadFetcher)
	got, err := p.GetThread(context.Background(), domain.SocialAccount{InstanceURL: srv.URL}, PublishAuth{AccessToken: "t"}, ThreadTarget{PublishedURL: "https://x/@a/1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != maxThreadReplies {
		t.Fatalf("len = %d, want %d", len(got), maxThreadReplies)
	}
}

func TestMastodonGetThread_BodyBounded(t *testing.T) {
	srv := newThreadServer(t, "t", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"descendants":[{"id":"2","content":"`))
		chunk := []byte(strings.Repeat("a", 1<<20))
		for i := 0; i < 6; i++ {
			w.Write(chunk)
		}
		w.Write([]byte(`"}]}`))
	})
	p := NewMastodonProvider(MastodonRegistrationConfig{}).(ThreadFetcher)
	if _, err := p.GetThread(context.Background(), domain.SocialAccount{InstanceURL: srv.URL}, PublishAuth{AccessToken: "t"}, ThreadTarget{PublishedURL: "https://x/@a/1"}); err == nil {
		t.Fatal("oversized body must fail")
	}
}
