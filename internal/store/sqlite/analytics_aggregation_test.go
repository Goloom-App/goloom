package sqlite

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"github.com/google/uuid"
)

func memoryStoreAgg(t *testing.T) *Store {
	t.Helper()
	enc, err := security.NewEncrypter("analytics-agg-test-secret-32bytes!!")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	s, err := New(ctx, dsn, enc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestGetTeamAnalyticsReport_deltaFromHistory(t *testing.T) {
	ctx := context.Background()
	s := memoryStoreAgg(t)
	u, err := s.UpsertOIDCUser(ctx, "agg", "agg@x", "Agg")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "agg-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "x", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}
	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content: "c", ScheduledAt: time.Now().UTC(), TargetAccounts: []string{acc.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPostMetrics(ctx, post.ID, acc.ID, map[string]int64{"likes": 10}); err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	if _, err := s.db.ExecContext(ctx, `
		insert into post_metrics_history (post_id, account_id, metric, value, recorded_at)
		values (?, ?, 'likes', 4, ?)
		on conflict(post_id, account_id, metric, recorded_at) do update set value = excluded.value`,
		post.ID, acc.ID, yesterday,
	); err != nil {
		t.Fatal(err)
	}

	report, err := s.GetTeamAnalyticsReport(ctx, team.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	var likes *domain.TeamMetricDelta
	for i := range report.Metrics {
		if report.Metrics[i].Metric == "likes" {
			likes = &report.Metrics[i]
			break
		}
	}
	if likes == nil {
		t.Fatalf("likes metric missing: %#v", report.Metrics)
	}
	if likes.Total != 10 {
		t.Fatalf("total: %d", likes.Total)
	}
	if likes.DeltaVsPrevDay != 6 {
		t.Fatalf("delta: %d", likes.DeltaVsPrevDay)
	}
}

func TestGetTeamMetricHistorySeries(t *testing.T) {
	ctx := context.Background()
	s := memoryStoreAgg(t)
	u, _ := s.UpsertOIDCUser(ctx, "ch", "ch@x", "Ch")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ch-" + uuid.NewString(), Description: ""})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "x", AccessToken: "t",
	})
	principal := domain.AuthenticatedPrincipal{User: u}
	post, _ := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content: "c", ScheduledAt: time.Now().UTC(), TargetAccounts: []string{acc.ID},
	})
	_ = s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil)
	_ = s.UpsertPostMetrics(ctx, post.ID, acc.ID, map[string]int64{"likes": 7})

	series, err := s.GetTeamMetricHistorySeries(ctx, team.ID, "likes", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) < 1 {
		t.Fatalf("series: %#v", series)
	}
	found := false
	for _, pt := range series {
		if pt.Value == 7 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected value 7 in series: %#v", series)
	}
}

func TestListTeamPostAnalyticsRanking_includesPostsWithoutMetrics(t *testing.T) {
	ctx := context.Background()
	s := memoryStoreAgg(t)
	u, _ := s.UpsertOIDCUser(ctx, "rank", "rank@x", "Rank")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "rank-" + uuid.NewString(), Description: ""})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "x", AccessToken: "t",
	})
	principal := domain.AuthenticatedPrincipal{User: u}

	withMetrics, _ := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Title: "With metrics", Content: "c", ScheduledAt: time.Now().UTC(), TargetAccounts: []string{acc.ID},
	})
	withoutMetrics, _ := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Title: "No metrics", Content: "c2", ScheduledAt: time.Now().UTC().Add(-time.Hour), TargetAccounts: []string{acc.ID},
	})
	_ = s.MarkPostResult(ctx, withMetrics.ID, 1, domain.PostStatusPosted, "", nil)
	_ = s.MarkPostResult(ctx, withoutMetrics.ID, 1, domain.PostStatusPosted, "", nil)
	_ = s.UpsertPostMetrics(ctx, withMetrics.ID, acc.ID, map[string]int64{"likes": 5})

	rows, err := s.ListTeamPostAnalyticsRanking(ctx, team.ID, "score", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: %#v", rows)
	}
	if rows[0].PostID != withMetrics.ID || rows[0].Score != 5 {
		t.Fatalf("expected scored post first: %#v", rows[0])
	}
	foundZero := false
	for _, row := range rows {
		if row.PostID == withoutMetrics.ID && row.Score == 0 {
			foundZero = true
		}
	}
	if !foundZero {
		t.Fatalf("expected post without metrics in ranking: %#v", rows)
	}

	metrics, err := s.ListPostMetricsForTeamPost(ctx, team.ID, withMetrics.ID)
	if err != nil || len(metrics) < 1 {
		t.Fatalf("metrics: %v %#v", err, metrics)
	}
	empty, err := s.ListPostMetricsForTeamPost(ctx, team.ID, withoutMetrics.ID)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty metrics: %v %#v", err, empty)
	}
}
