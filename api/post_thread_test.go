package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

type threadSeed struct {
	token, teamID, postID, accountID string
}

// seedThreadPost creates a team, an account of the given provider/instance and a post targeting it.
// publishedURL == "" leaves the target unpublished.
func seedThreadPost(t *testing.T, s *sqlitestore.Store, providerName, instanceURL, publishedURL string) threadSeed {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "api-th-"+uuid.NewString(), "th@example.test", "Thread API")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "team-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	plain, _, err := s.CreateUserAPIToken(ctx, u.ID, "integration", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	authType := domain.AccountAuthTypeOAuthToken
	if providerName == "bluesky" {
		authType = domain.AccountAuthTypeAppPassword
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: providerName, AuthType: authType, InstanceURL: instanceURL, Username: "x", AccessToken: "secret-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: u}, domain.CreatePostInput{
		Content: "hello", ScheduledAt: time.Now().UTC(), TargetAccounts: []string{acc.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if publishedURL != "" {
		remoteID := publishedURL[strings.LastIndex(publishedURL, "/")+1:]
		if remoteID == "" || strings.Trim(remoteID, "0123456789") != "" {
			remoteID = "10" // e.g. Friendica /display/<guid>
		}
		if err := s.MarkPostTargetResult(ctx, post.ID, acc.ID, domain.PostStatusPosted, publishedURL, "", nil, remoteID); err != nil {
			t.Fatal(err)
		}
	}
	return threadSeed{plain, team.ID, post.ID, acc.ID}
}

func threadTestHandler(t *testing.T, st store.Store) http.Handler {
	t.Helper()
	authSvc, err := auth.New(context.Background(), config.Config{}, st)
	if err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry(
		provider.NewBlueskyProvider(),
		provider.NewFriendicaProvider(),
		provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}),
	)
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AllowPrivateProviderInstanceURLs: true}
	h := api.New(slog.New(slog.NewTextHandler(io.Discard, nil)), st, authSvc, reg, cfg, nil, catalog, nil, nil)
	return h.Handler(security.NewLimiter(10_000, 10_000), nil)
}

// failingTargetStore makes GetPublishedTarget fail with a non-not-found error.
type failingTargetStore struct{ store.Store }

func (failingTargetStore) GetPublishedTarget(context.Context, string, string) (domain.PublishedTarget, error) {
	return domain.PublishedTarget{}, errors.New("db down")
}

func getThread(t *testing.T, h http.Handler, token, teamID, postID, accountID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/posts/"+postID+"/targets/"+accountID+"/thread", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAPI_PostThread(t *testing.T) {
	var hits40, hits99 int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("remote auth header: %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/statuses/10/context":
			fmt.Fprint(w, `{"descendants":[{"id":"11","in_reply_to_id":"10","url":"https://m/@a/11","content":"<p>hi</p>","created_at":"2026-01-02T03:04:05Z","visibility":"public","account":{"acct":"a","display_name":"A","avatar":"https://m/a.png","url":"https://m/@a"}}]}`)
		case "/api/v1/statuses/30/context":
			fmt.Fprint(w, `{"descendants":[`+strings.Repeat(`{"id":"1","content":"x","created_at":"2026-01-02T03:04:05Z","visibility":"public","account":{"acct":"a"}},`, 199)+`{"id":"2","content":"x","created_at":"2026-01-02T03:04:05Z","visibility":"public","spoiler_text":"cw","sensitive":true,"account":{"acct":"a"}},{"id":"3","content":"x","visibility":"public","account":{"acct":"a"}}]}`)
		case "/api/v1/statuses/40/context":
			atomic.AddInt32(&hits40, 1)
			fmt.Fprint(w, `{"descendants":[]}`)
		case "/api/v1/statuses/20/context":
			fmt.Fprint(w, `{"descendants":[]}`)
		default:
			if r.URL.Path == "/api/v1/statuses/99/context" {
				atomic.AddInt32(&hits99, 1)
			}
			http.Error(w, "upstream says no, token secret-token", http.StatusInternalServerError)
		}
	}))
	defer remote.Close()

	s := newMemorySQLite(t)
	h := threadTestHandler(t, s)

	t.Run("200 with replies", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/10")
		rec := getThread(t, h, sd.token, sd.teamID, sd.postID, sd.accountID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		var body struct {
			PostID       string `json:"post_id"`
			AccountID    string `json:"account_id"`
			Provider     string `json:"provider"`
			PublishedURL string `json:"published_url"`
			FetchedAt    string `json:"fetched_at"`
			Replies      []struct {
				RemoteID    string `json:"remote_id"`
				ParentID    string `json:"parent_id"`
				URL         string `json:"url"`
				Content     string `json:"content"`
				PublishedAt string `json:"published_at"`
				Author      struct {
					Handle      string `json:"handle"`
					DisplayName string `json:"display_name"`
					AvatarURL   string `json:"avatar_url"`
					ProfileURL  string `json:"profile_url"`
				} `json:"author"`
			} `json:"replies"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.PostID != sd.postID || body.AccountID != sd.accountID || body.Provider != "mastodon" || body.PublishedURL != "https://m/@x/10" {
			t.Errorf("envelope: %#v", body)
		}
		if _, err := time.Parse(time.RFC3339, body.FetchedAt); err != nil {
			t.Errorf("fetched_at: %v", err)
		}
		if len(body.Replies) != 1 {
			t.Fatalf("replies: %#v", body.Replies)
		}
		r := body.Replies[0]
		if r.RemoteID != "11" || r.ParentID != "10" || r.Content != "hi" || r.Author.Handle != "a" || r.Author.AvatarURL != "https://m/a.png" || r.PublishedAt != "2026-01-02T03:04:05Z" {
			t.Errorf("reply: %#v", r)
		}
	})

	t.Run("200 friendica guid url uses stored remote id", func(t *testing.T) {
		sd := seedThreadPost(t, s, "friendica", remote.URL, "https://f.example/display/0123456789abcdef0123456789abcdef")
		rec := getThread(t, h, sd.token, sd.teamID, sd.postID, sd.accountID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("200 empty replies is [] not null", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/20")
		rec := getThread(t, h, sd.token, sd.teamID, sd.postID, sd.accountID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(rec.Body.Bytes(), &raw)
		if string(raw["replies"]) != "[]" {
			t.Fatalf("replies = %s", raw["replies"])
		}
	})

	t.Run("401 without token", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/10")
		if rec := getThread(t, h, "", sd.teamID, sd.postID, sd.accountID); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("403 for non-member", func(t *testing.T) {
		a := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/10")
		b := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/10")
		if rec := getThread(t, h, b.token, a.teamID, a.postID, a.accountID); rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("404 unknown post", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/10")
		rec := getThread(t, h, sd.token, sd.teamID, uuid.NewString(), sd.accountID)
		if rec.Code != http.StatusNotFound || rec.Header().Get("X-Error-Code") != "not_found" {
			t.Fatalf("status %d code %q", rec.Code, rec.Header().Get("X-Error-Code"))
		}
	})

	t.Run("404 post of another team", func(t *testing.T) {
		a := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/10")
		b := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/10")
		if rec := getThread(t, h, b.token, b.teamID, a.postID, a.accountID); rec.Code != http.StatusNotFound {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("404 account not targeted", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/10")
		if rec := getThread(t, h, sd.token, sd.teamID, sd.postID, uuid.NewString()); rec.Code != http.StatusNotFound {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("409 target not published", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "")
		rec := getThread(t, h, sd.token, sd.teamID, sd.postID, sd.accountID)
		if rec.Code != http.StatusConflict || rec.Header().Get("X-Error-Code") != "post_not_published" {
			t.Fatalf("status %d code %q", rec.Code, rec.Header().Get("X-Error-Code"))
		}
	})

	t.Run("422 provider without thread support", func(t *testing.T) {
		sd := seedThreadPost(t, s, "bluesky", "https://bsky.social", "https://bsky.app/profile/x/post/abc")
		rec := getThread(t, h, sd.token, sd.teamID, sd.postID, sd.accountID)
		if rec.Code != http.StatusUnprocessableEntity || rec.Header().Get("X-Error-Code") != "thread_unsupported" {
			t.Fatalf("status %d code %q", rec.Code, rec.Header().Get("X-Error-Code"))
		}
	})

	t.Run("200 truncated flag and content warning", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/30")
		rec := getThread(t, h, sd.token, sd.teamID, sd.postID, sd.accountID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Truncated bool `json:"truncated"`
			Replies   []struct {
				ContentWarning string `json:"content_warning"`
				Sensitive      bool   `json:"sensitive"`
			} `json:"replies"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if !body.Truncated || len(body.Replies) != 200 || body.Replies[199].ContentWarning != "cw" || !body.Replies[199].Sensitive {
			t.Fatalf("truncated=%v len=%d", body.Truncated, len(body.Replies))
		}
	})

	t.Run("409 unresolvable target", func(t *testing.T) {
		sd := seedThreadPost(t, s, "friendica", remote.URL, "")
		if err := s.MarkPostTargetResult(context.Background(), sd.postID, sd.accountID, domain.PostStatusPosted, "https://f/display/abc", "", nil, ""); err != nil {
			t.Fatal(err)
		}
		rec := getThread(t, h, sd.token, sd.teamID, sd.postID, sd.accountID)
		if rec.Code != http.StatusConflict || rec.Header().Get("X-Error-Code") != "thread_target_unresolvable" {
			t.Fatalf("status %d code %q", rec.Code, rec.Header().Get("X-Error-Code"))
		}
	})

	t.Run("500 on store error", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/10")
		hf := threadTestHandler(t, failingTargetStore{s})
		rec := getThread(t, hf, sd.token, sd.teamID, sd.postID, sd.accountID)
		if rec.Code != http.StatusInternalServerError || rec.Header().Get("X-Error-Code") != "internal_error" {
			t.Fatalf("status %d code %q", rec.Code, rec.Header().Get("X-Error-Code"))
		}
	})

	t.Run("cache: repeated requests hit upstream once", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/40")
		for i := 0; i < 3; i++ {
			if rec := getThread(t, h, sd.token, sd.teamID, sd.postID, sd.accountID); rec.Code != http.StatusOK {
				t.Fatalf("status %d", rec.Code)
			}
		}
		if got := atomic.LoadInt32(&hits40); got != 1 {
			t.Fatalf("upstream calls = %d, want 1", got)
		}
	})

	t.Run("errors are not cached", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/99")
		for i := 0; i < 2; i++ {
			if rec := getThread(t, h, sd.token, sd.teamID, sd.postID, sd.accountID); rec.Code != http.StatusBadGateway {
				t.Fatalf("status %d", rec.Code)
			}
		}
		if got := atomic.LoadInt32(&hits99); got != 2 {
			t.Fatalf("upstream calls = %d, want 2", got)
		}
	})

	t.Run("502 upstream failure without leaking token", func(t *testing.T) {
		sd := seedThreadPost(t, s, "mastodon", remote.URL, "https://m/@x/99")
		rec := getThread(t, h, sd.token, sd.teamID, sd.postID, sd.accountID)
		if rec.Code != http.StatusBadGateway || rec.Header().Get("X-Error-Code") != "thread_fetch_failed" {
			t.Fatalf("status %d code %q", rec.Code, rec.Header().Get("X-Error-Code"))
		}
		if b := rec.Body.String(); len(b) == 0 || strings.Contains(b, "secret-token") || strings.Contains(b, "upstream") {
			t.Fatalf("body leaks upstream detail: %q", b)
		}
	})
}
