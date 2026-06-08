package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestSQLite_ExternalPostMonitor_CreateImportedPostDedup(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	owner, err := s.UpsertOIDCUser(ctx, "ext-import-owner", "owner@example.com", "Owner")
	if err != nil {
		t.Fatalf("UpsertOIDCUser: %v", err)
	}
	team, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "ext-import-team", Description: ""})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider:    "mastodon",
		AuthType:    domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://mastodon.example",
		Username:    "user",
		AccessToken: "plain",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	settings, err := s.UpsertExternalPostMonitorSettings(ctx, team.ID, domain.UpsertExternalPostMonitorInput{Enabled: true})
	if err != nil {
		t.Fatalf("UpsertExternalPostMonitorSettings: %v", err)
	}
	if !settings.Enabled {
		t.Fatal("expected enabled")
	}

	publishedAt := time.Now().UTC().Add(-2 * time.Hour)
	input := domain.ImportedPostInput{
		AccountID:       acc.ID,
		RemotePostID:    "remote-status-99",
		Content:         "Imported content",
		PublishedAt:     publishedAt,
		PublishedURL:    "https://example.com/statuses/99",
		PublishMetadata: map[string]string{"uri": "https://example.com/users/u/statuses/99"},
	}
	post, err := s.CreateImportedPost(ctx, team.ID, owner.ID, input)
	if err != nil {
		t.Fatalf("CreateImportedPost: %v", err)
	}
	if post.Status != domain.PostStatusPosted || post.Source != domain.PostSourceImported {
		t.Fatalf("unexpected post: status=%s source=%s", post.Status, post.Source)
	}

	exists, err := s.AuthorPostAlreadyTracked(ctx, acc.ID, "remote-status-99", input.PublishedURL, input.PublishMetadata)
	if err != nil || !exists {
		t.Fatalf("AuthorPostAlreadyTracked numeric id: exists=%v err=%v", exists, err)
	}

	exists, err = s.AuthorPostAlreadyTracked(ctx, acc.ID, "https://example.com/users/u/statuses/99", input.PublishedURL, input.PublishMetadata)
	if err != nil || !exists {
		t.Fatalf("AuthorPostAlreadyTracked uri alias: exists=%v err=%v", exists, err)
	}

	_, err = s.CreateImportedPost(ctx, team.ID, owner.ID, input)
	if err == nil {
		t.Fatal("expected duplicate import to fail")
	}
}

func TestSQLite_DeleteRedundantImportedPosts_removesImportedDuplicate(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	owner, err := s.UpsertOIDCUser(ctx, "dedup-owner", "dedup@example.com", "Owner")
	if err != nil {
		t.Fatalf("UpsertOIDCUser: %v", err)
	}
	team, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "dedup-team", Description: ""})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider:    "mastodon",
		AuthType:    domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://mastodon.example",
		Username:    "user",
		AccessToken: "plain",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	publishedAt := time.Now().UTC().Add(-time.Hour)
	principal := domain.AuthenticatedPrincipal{User: owner}
	goloomPost, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content:        "Published via goloom",
		ScheduledAt:    publishedAt,
		TargetAccounts: []string{acc.ID},
	})
	if err != nil {
		t.Fatalf("CreateScheduledPost: %v", err)
	}
	if err := s.MarkPostResult(ctx, goloomPost.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
		t.Fatalf("MarkPostResult: %v", err)
	}
	uri := "https://mastodon.example/users/user/statuses/555"
	if err := s.MarkPostTargetResult(ctx, goloomPost.ID, acc.ID, domain.PostStatusPosted, "https://mastodon.example/@user/555", "", map[string]string{"uri": uri}, uri); err != nil {
		t.Fatalf("MarkPostTargetResult: %v", err)
	}

	imported, err := s.CreateImportedPost(ctx, team.ID, owner.ID, domain.ImportedPostInput{
		AccountID:       acc.ID,
		RemotePostID:    "555",
		Content:         "Published via goloom",
		PublishedAt:     publishedAt,
		PublishedURL:    "https://mastodon.example/@user/555",
		PublishMetadata: map[string]string{"uri": uri},
	})
	if err != nil {
		t.Fatalf("CreateImportedPost duplicate: %v", err)
	}

	removed, err := s.DeleteRedundantImportedPosts(ctx, team.ID)
	if err != nil {
		t.Fatalf("DeleteRedundantImportedPosts: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d want 1", removed)
	}

	if _, err := s.GetScheduledPost(ctx, team.ID, imported.ID); err == nil {
		t.Fatal("expected imported duplicate to be deleted")
	}
	if _, err := s.GetScheduledPost(ctx, team.ID, goloomPost.ID); err != nil {
		t.Fatalf("expected goloom post to remain: %v", err)
	}
}
