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

	exists, err := s.TargetExistsByRemotePostID(ctx, acc.ID, "remote-status-99")
	if err != nil || !exists {
		t.Fatalf("TargetExistsByRemotePostID: exists=%v err=%v", exists, err)
	}

	_, err = s.CreateImportedPost(ctx, team.ID, owner.ID, input)
	if err == nil {
		t.Fatal("expected duplicate import to fail")
	}
}
