package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

// ctxFor returns a tool-handler context carrying the principal for the given scopes.
func (f mcpFixture) ctxFor(t *testing.T, scopes string) context.Context {
	t.Helper()
	principal, err := f.store.LookupAPIToken(context.Background(), f.apiToken(t, scopes))
	if err != nil {
		t.Fatalf("LookupAPIToken: %v", err)
	}
	return WithPrincipal(context.Background(), principal)
}

// blueskyAccount adds a Bluesky account (300-char limit) to the fixture's team.
func (f mcpFixture) blueskyAccount(t *testing.T) domain.SocialAccount {
	t.Helper()
	acc, err := f.store.CreateAccount(context.Background(), f.team.ID, domain.ConnectedAccount{
		Provider: "bluesky", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://bsky.social", Username: "bsky-" + uuid.NewString(), AccessToken: "tok",
	})
	if err != nil {
		t.Fatalf("CreateAccount bluesky: %v", err)
	}
	return acc
}

// foreignAccount creates a second team (same owner) with its own account, to
// exercise cross-team targeting protection.
func (f mcpFixture) foreignAccount(t *testing.T) domain.SocialAccount {
	t.Helper()
	other, err := f.store.CreateTeam(context.Background(), f.user.ID, domain.CreateTeamInput{Name: "other-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	acc, err := f.store.CreateAccount(context.Background(), other.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m.example", Username: "foreign", AccessToken: "tok",
	})
	if err != nil {
		t.Fatalf("CreateAccount foreign: %v", err)
	}
	return acc
}

func soon() string { return time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339) }

// ===== schedule_post =====

func TestSchedulePost_RejectsOversizedContent(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	_, _, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Test post",
		Content:        strings.Repeat("x", 400), // > 300 (bluesky)
		ScheduledAt:    soon(),
		TargetAccounts: []string{bsky.ID},
	})
	if err == nil {
		t.Fatal("oversized content must be rejected")
	}
	posts, _ := f.store.ListTeamPosts(context.Background(), f.team.ID)
	if len(posts) != 0 {
		t.Fatalf("no post must be saved when validation fails, got %d", len(posts))
	}
}

func TestSchedulePost_AppliesValidOverride(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	bsky := f.blueskyAccount(t)
	long := strings.Repeat("x", 400)

	_, out, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:                 f.team.ID,
		Title:                  "Test post",
		Content:                long, // too long for bluesky on its own
		ScheduledAt:            soon(),
		TargetAccounts:         []string{bsky.ID},
		AccountContentOverride: map[string]string{bsky.ID: "short text for bluesky"},
	})
	if err != nil {
		t.Fatalf("valid override must be accepted: %v", err)
	}
	versions, err := f.store.ListPostVersionsForTeamPost(context.Background(), f.team.ID, out.PostID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].AccountID != bsky.ID || versions[0].Content != "short text for bluesky" {
		t.Fatalf("override must be persisted as a post version, got %+v", versions)
	}
}

func TestSchedulePost_OverrideKeyMismatchRejected(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	_, _, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:                 f.team.ID,
		Title:                  "Test post",
		Content:                "fits everywhere",
		ScheduledAt:            soon(),
		TargetAccounts:         []string{bsky.ID},
		AccountContentOverride: map[string]string{"some-other-id": "ignored"},
	})
	if err == nil {
		t.Fatal("override for a non-target account must be rejected, not silently dropped")
	}
}

func TestSchedulePost_CrossTeamTargetRejected(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	foreign := f.foreignAccount(t)

	_, _, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Test post",
		Content:        "hello",
		ScheduledAt:    soon(),
		TargetAccounts: []string{foreign.ID},
	})
	if err == nil {
		t.Fatal("targeting another team's account must be rejected")
	}
	if !strings.Contains(err.Error(), "does not belong to team") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchedulePost_UnknownTargetRejected(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)

	_, _, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Test post",
		Content:        "hello",
		ScheduledAt:    soon(),
		TargetAccounts: []string{uuid.NewString()},
	})
	if err == nil {
		t.Fatal("unknown target account must be rejected")
	}
}

func TestSchedulePost_EmptyContentRejected(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)

	_, _, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Test post",
		Content:        "   ",
		ScheduledAt:    soon(),
		TargetAccounts: []string{f.account.ID},
	})
	if err == nil {
		t.Fatal("empty content must be rejected")
	}
}

func TestSchedulePost_EmptyTargetsRejected(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)

	_, _, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:      f.team.ID,
		Title:       "Test post",
		Content:     "hello",
		ScheduledAt: soon(),
	})
	if err == nil {
		t.Fatal("missing target_accounts must be rejected")
	}
}

func TestSchedulePost_RequiresTitle(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)

	_, _, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "   ",
		Content:        "hello",
		ScheduledAt:    soon(),
		TargetAccounts: []string{f.account.ID},
	})
	if err == nil {
		t.Fatal("schedule_post must require an explicit title")
	}
}

func TestSchedulePost_RequiresTeam(t *testing.T) {
	f := newMCPFixture(t)
	bsky := f.blueskyAccount(t)
	// An admin passes the team-access check even for an empty team, so the
	// handler itself must reject an empty team_id (via the pipeline's RequireTeam)
	// so a post can never be persisted with an inferred/empty team.
	admin := domain.AuthenticatedPrincipal{User: domain.User{ID: f.user.ID, IsAdmin: true}}
	ctx := WithPrincipal(context.Background(), admin)

	_, _, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         "",
		Title:          "T",
		Content:        "hello",
		ScheduledAt:    soon(),
		TargetAccounts: []string{bsky.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "team_id") {
		t.Fatalf("schedule_post must reject an empty team_id, got %v", err)
	}
}

// ===== draft_post =====

func TestDraftPost_AllowsOversizedButValidatesTargets(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	// Drafts may exceed limits (refined before scheduling).
	if _, _, err := f.handler.handleDraftPost(ctx, nil, DraftPostInput{
		TeamID:         f.team.ID,
		Title:          "Test post",
		Content:        strings.Repeat("x", 400),
		TargetAccounts: []string{bsky.ID},
	}); err != nil {
		t.Fatalf("oversized draft must be allowed: %v", err)
	}

	// But cross-team targets are still rejected.
	foreign := f.foreignAccount(t)
	if _, _, err := f.handler.handleDraftPost(ctx, nil, DraftPostInput{
		TeamID:         f.team.ID,
		Title:          "Test post",
		Content:        "hi",
		TargetAccounts: []string{foreign.ID},
	}); err == nil {
		t.Fatal("draft targeting another team's account must be rejected")
	}
}

func TestDraftPost_OverrideKeyMismatchRejected(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)

	if _, _, err := f.handler.handleDraftPost(ctx, nil, DraftPostInput{
		TeamID:                 f.team.ID,
		Title:                  "Test post",
		Content:                "hi",
		TargetAccounts:         []string{f.account.ID},
		AccountContentOverride: map[string]string{"nope": "x"},
	}); err == nil {
		t.Fatal("draft override for a non-target account must be rejected")
	}
}

func TestDraftPost_RequiresTitle(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)

	if _, _, err := f.handler.handleDraftPost(ctx, nil, DraftPostInput{
		TeamID:         f.team.ID,
		Content:        "hello",
		TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("draft_post must require an explicit title")
	}
}

// ===== modify_post =====

func TestModifyPost_RejectsOversizedContentOnScheduled(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	_, created, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Test post",
		Content:        "short",
		ScheduledAt:    soon(),
		TargetAccounts: []string{bsky.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	long := strings.Repeat("y", 400)
	if _, _, err := f.handler.handleModifyPost(ctx, nil, ModifyPostInput{
		TeamID:  f.team.ID,
		PostID:  created.PostID,
		Content: &long,
	}); err == nil {
		t.Fatal("modifying to oversized content must be rejected")
	}
	post, _ := f.store.GetScheduledPost(context.Background(), f.team.ID, created.PostID)
	if post.Content != "short" {
		t.Fatalf("content must be unchanged after rejected modify, got %q", post.Content)
	}
}

func TestModifyPost_AppliesOverride(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	_, created, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Test post",
		Content:        "short",
		ScheduledAt:    soon(),
		TargetAccounts: []string{bsky.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := f.handler.handleModifyPost(ctx, nil, ModifyPostInput{
		TeamID:                 f.team.ID,
		PostID:                 created.PostID,
		AccountContentOverride: map[string]string{bsky.ID: "bluesky-specific text"},
	}); err != nil {
		t.Fatalf("modify with valid override must succeed: %v", err)
	}
	versions, _ := f.store.ListPostVersionsForTeamPost(context.Background(), f.team.ID, created.PostID)
	if len(versions) != 1 || versions[0].Content != "bluesky-specific text" {
		t.Fatalf("modify must persist the override (was previously ignored), got %+v", versions)
	}
}

func TestModifyPost_AppliesTargetChange(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	_, created, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Test post",
		Content:        "short",
		ScheduledAt:    soon(),
		TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	newTargets := []string{bsky.ID}
	if _, _, err := f.handler.handleModifyPost(ctx, nil, ModifyPostInput{
		TeamID:         f.team.ID,
		PostID:         created.PostID,
		TargetAccounts: &newTargets,
	}); err != nil {
		t.Fatalf("modify targets must succeed: %v", err)
	}
	post, _ := f.store.GetScheduledPost(context.Background(), f.team.ID, created.PostID)
	if len(post.TargetAccounts) != 1 || post.TargetAccounts[0] != bsky.ID {
		t.Fatalf("targets must be updated (was previously ignored), got %+v", post.TargetAccounts)
	}
}

func TestModifyPost_RejectsEmptyTitle(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)

	_, created, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Original",
		Content:        "short",
		ScheduledAt:    soon(),
		TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	empty := "  "
	if _, _, err := f.handler.handleModifyPost(ctx, nil, ModifyPostInput{
		TeamID: f.team.ID,
		PostID: created.PostID,
		Title:  &empty,
	}); err == nil {
		t.Fatal("modify_post must reject clearing the title to empty")
	}
}

func TestModifyPost_CrossTeamTargetRejected(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	foreign := f.foreignAccount(t)

	_, created, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Test post",
		Content:        "short",
		ScheduledAt:    soon(),
		TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	bad := []string{foreign.ID}
	if _, _, err := f.handler.handleModifyPost(ctx, nil, ModifyPostInput{
		TeamID:         f.team.ID,
		PostID:         created.PostID,
		TargetAccounts: &bad,
	}); err == nil {
		t.Fatal("modify to a cross-team target must be rejected")
	}
}

// ===== create_recurring =====

func TestCreateRecurring_RejectsCrossTeamTarget(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	foreign := f.foreignAccount(t)

	if _, _, err := f.handler.handleCreateRecurring(ctx, nil, CreateRecurringInput{
		TeamID:         f.team.ID,
		Title:          "t",
		Content:        "c",
		RecurrenceJSON: `{"freq":"WEEKLY","interval":1,"byday":["TU"]}`,
		TargetAccounts: []string{foreign.ID},
	}); err == nil {
		t.Fatal("recurring with a cross-team target must be rejected")
	}
}

func TestCreateRecurring_RequiresContent(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)

	if _, _, err := f.handler.handleCreateRecurring(ctx, nil, CreateRecurringInput{
		TeamID:         f.team.ID,
		Title:          "t",
		Content:        "  ",
		RecurrenceJSON: `{"freq":"WEEKLY","interval":1,"byday":["TU"]}`,
		TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("recurring without content must be rejected")
	}
}

// ===== create_rss_feed =====

func TestCreateRSSFeed_RejectsInvalidURL(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)

	for _, bad := range []string{"", "not a url", "ftp://example.com/feed"} {
		if _, _, err := f.handler.handleCreateRSSFeed(ctx, nil, CreateRSSFeedInput{
			TeamID:           f.team.ID,
			FeedURL:          bad,
			Name:             "feed",
			TargetAccountIDs: []string{f.account.ID},
		}); err == nil {
			t.Fatalf("feed_url %q must be rejected", bad)
		}
	}
}

func TestCreateRSSFeed_RejectsCrossTeamTarget(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)
	foreign := f.foreignAccount(t)

	if _, _, err := f.handler.handleCreateRSSFeed(ctx, nil, CreateRSSFeedInput{
		TeamID:           f.team.ID,
		FeedURL:          "https://example.com/feed.xml",
		Name:             "feed",
		TargetAccountIDs: []string{foreign.ID},
	}); err == nil {
		t.Fatal("rss feed with a cross-team target must be rejected")
	}
}

// ===== create_campaign =====

func TestCreateCampaign_RequiresName(t *testing.T) {
	f := newMCPFixture(t)
	ctx := f.ctxFor(t, `["write"]`)

	if _, _, err := f.handler.handleCreateCampaign(ctx, nil, CreateCampaignInput{
		TeamID: f.team.ID,
		Name:   "   ",
	}); err == nil {
		t.Fatal("campaign without a name must be rejected")
	}
}

// ===== scope enforcement across write tools =====

func TestWriteToolsRequireScopes(t *testing.T) {
	f := newMCPFixture(t)
	readOnly := f.ctxFor(t, `["read"]`)

	if _, _, err := f.handler.handleSchedulePost(readOnly, nil, SchedulePostInput{
		TeamID: f.team.ID, Content: "x", ScheduledAt: soon(), TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("schedule_post must require a write scope")
	}
	if _, _, err := f.handler.handleDeletePost(readOnly, nil, DeletePostInput{
		TeamID: f.team.ID, PostID: uuid.NewString(),
	}); err == nil {
		t.Fatal("delete_post must require the delete scope")
	}
	if _, _, err := f.handler.handleCreateCampaign(readOnly, nil, CreateCampaignInput{
		TeamID: f.team.ID, Name: "c",
	}); err == nil {
		t.Fatal("create_campaign must require a write scope")
	}
}
