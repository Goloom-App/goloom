package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
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
	var calls int
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("remote auth header: %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/statuses/10/context":
			fmt.Fprint(w, `{"descendants":[{"id":"11","in_reply_to_id":"10","url":"https://m/@a/11","content":"<p>hi</p>","created_at":"2026-01-02T03:04:05Z","visibility":"public","account":{"acct":"a","display_name":"A","avatar":"https://m/a.png","url":"https://m/@a"}}]}`)
		case "/api/v1/statuses/20/context":
			fmt.Fprint(w, `{"descendants":[]}`)
		default:
			http.Error(w, "upstream says no, token secret-token", http.StatusInternalServerError)
		}
	}))
	defer remote.Close()

	s := newMemorySQLite(t)
	h := analyticsTestHandler(t, s)

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
