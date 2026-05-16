package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func newMemorySQLite(t *testing.T) *sqlitestore.Store {
	t.Helper()
	enc, err := security.NewEncrypter("api-analytics-test-secret-32bytes!!")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	s, err := sqlitestore.New(ctx, dsn, enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedTeamWithPostedMetrics returns owner API token, team id, posted post id.
func seedTeamWithPostedMetrics(t *testing.T, s *sqlitestore.Store) (bearer string, teamID string, postID string) {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "api-an-"+uuid.NewString(), "an@example.test", "Analytics API")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	plain, _, err := s.CreateUserAPIToken(ctx, u.ID, "integration", nil)
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://social.example", Username: "x", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}
	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content: "hello", ScheduledAt: time.Now().UTC(), TargetAccounts: []string{acc.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPostMetrics(ctx, post.ID, acc.ID, map[string]int64{"likes": 5, "reposts": 2}); err != nil {
		t.Fatal(err)
	}
	return plain, team.ID, post.ID
}

func analyticsTestHandler(t *testing.T, s *sqlitestore.Store) http.Handler {
	t.Helper()
	ctx := context.Background()
	authSvc, err := auth.New(ctx, config.Config{}, s)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	reg := provider.NewRegistry(
		provider.NewBlueskyProvider(),
		provider.NewFriendicaProvider(),
		provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}),
	)
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	h := api.New(logger, s, authSvc, reg, config.Config{})
	return h.Handler(security.NewLimiter(10_000, 10_000), nil)
}

func TestAPI_TeamAnalytics_and_PostAnalytics(t *testing.T) {
	s := newMemorySQLite(t)
	token, teamID, postID := seedTeamWithPostedMetrics(t, s)
	h := analyticsTestHandler(t, s)

	t.Run("team analytics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/analytics", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		var body struct {
			MetricsTotal map[string]int64 `json:"metrics_total"`
			TopPosts     []any            `json:"top_posts"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.MetricsTotal["likes"] != 5 || body.MetricsTotal["reposts"] != 2 {
			t.Fatalf("metrics_total: %#v", body.MetricsTotal)
		}
		if len(body.TopPosts) != 1 {
			t.Fatalf("top_posts len: %d", len(body.TopPosts))
		}
	})

	t.Run("post analytics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/posts/"+postID+"/analytics", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Items []domain.PostMetric `json:"items"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Items) < 2 {
			t.Fatalf("items: %#v", body.Items)
		}
	})

	t.Run("post analytics not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/posts/"+uuid.NewString()+"/analytics", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("unauthorized without token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/analytics", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", rec.Code)
		}
	})

	t.Run("analytics summary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/analytics/summary", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Metrics []struct {
				Metric         string `json:"metric"`
				Total          int64  `json:"total"`
				DeltaVsPrevDay int64  `json:"delta_vs_prev_day"`
			} `json:"metrics"`
			TopPosts []any `json:"top_posts"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Metrics) < 2 {
			t.Fatalf("metrics: %#v", body.Metrics)
		}
	})

	t.Run("analytics posts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/analytics/posts?limit=10", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Items []struct {
				PostID string `json:"post_id"`
				Score  int64  `json:"score"`
			} `json:"items"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Items) != 1 || body.Items[0].Score != 7 {
			t.Fatalf("items: %#v", body.Items)
		}
	})

	t.Run("post analytics empty for posted post without metrics", func(t *testing.T) {
		ctx := context.Background()
		u, err := s.UpsertOIDCUser(ctx, "empty-"+uuid.NewString(), "empty@example.test", "Empty")
		if err != nil {
			t.Fatal(err)
		}
		team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "empty-" + uuid.NewString(), Description: ""})
		if err != nil {
			t.Fatal(err)
		}
		plain, _, err := s.CreateUserAPIToken(ctx, u.ID, "empty-metrics", nil)
		if err != nil {
			t.Fatal(err)
		}
		acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
			Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
			InstanceURL: "https://social.example", Username: "x", AccessToken: "t",
		})
		if err != nil {
			t.Fatal(err)
		}
		principal := domain.AuthenticatedPrincipal{User: u}
		post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
			Content: "no metrics yet", ScheduledAt: time.Now().UTC(), TargetAccounts: []string{acc.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+team.ID+"/posts/"+post.ID+"/analytics", nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Items []domain.PostMetric `json:"items"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Items) != 0 {
			t.Fatalf("expected empty items, got %#v", body.Items)
		}

		listReq := httptest.NewRequest(http.MethodGet, "/v1/teams/"+team.ID+"/analytics/posts?limit=10", nil)
		listReq.Header.Set("Authorization", "Bearer "+plain)
		listRec := httptest.NewRecorder()
		h.ServeHTTP(listRec, listReq)
		if listRec.Code != http.StatusOK {
			t.Fatalf("list status %d body %s", listRec.Code, listRec.Body.String())
		}
		var listBody struct {
			Items []struct {
				PostID string `json:"post_id"`
				Score  int64  `json:"score"`
			} `json:"items"`
		}
		if err := json.NewDecoder(listRec.Body).Decode(&listBody); err != nil {
			t.Fatal(err)
		}
		if len(listBody.Items) != 1 || listBody.Items[0].PostID != post.ID {
			t.Fatalf("list items: %#v", listBody.Items)
		}
	})

	t.Run("analytics chart", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/analytics/chart?metric=likes&days=14", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Series []struct {
				Date  string `json:"date"`
				Value int64  `json:"value"`
			} `json:"series"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Series) < 1 {
			t.Fatalf("series: %#v", body.Series)
		}
	})

	t.Run("analytics chart missing metric", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/analytics/chart", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", rec.Code)
		}
	})
}

func TestAPI_TeamAnalytics_forbiddenOtherTeam(t *testing.T) {
	s := newMemorySQLite(t)
	_, teamID, _ := seedTeamWithPostedMetrics(t, s)

	ctx := context.Background()
	other, err := s.UpsertOIDCUser(ctx, "other-"+uuid.NewString(), "other@example.test", "Other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTeam(ctx, other.ID, domain.CreateTeamInput{Name: "other-" + uuid.NewString(), Description: ""}); err != nil {
		t.Fatal(err)
	}
	otherToken, _, err := s.CreateUserAPIToken(ctx, other.ID, "t2", nil)
	if err != nil {
		t.Fatal(err)
	}

	h := analyticsTestHandler(t, s)
	req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+teamID+"/analytics", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body %q", rec.Code, rec.Body.String())
	}
}
