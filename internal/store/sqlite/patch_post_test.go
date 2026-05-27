package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func TestSQLite_PatchScheduledPost_ScheduledAtOnlyPreservesVersions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "patch", "patch@x", "Patch")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "patch-" + uuid.NewString(), Description: ""})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "bluesky", AuthType: domain.AccountAuthTypeAppPassword,
		InstanceURL: "https://bsky.social", Username: "bsky", AccessToken: "tok",
	})
	principal := domain.AuthenticatedPrincipal{User: u}
	when := time.Now().UTC().Add(24 * time.Hour)
	long := string(make([]rune, 419))
	short := string(make([]rune, 232))

	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Title: "T", Content: long, ScheduledAt: when, TargetAccounts: []string{acc.ID},
		AccountContentOverride: map[string]string{acc.ID: short},
	})
	if err != nil {
		t.Fatal(err)
	}

	newWhen := when.Add(2 * time.Hour)
	_, err = s.PatchScheduledPost(ctx, team.ID, post.ID, domain.UpdatePostPatch{
		ScheduledAt: domain.PatchField[time.Time]{Set: true, Value: newWhen},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetScheduledPost(ctx, team.ID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ScheduledAt.Equal(newWhen) {
		t.Fatalf("scheduled_at: got %v want %v", got.ScheduledAt, newWhen)
	}
	if got.Content != long {
		t.Fatalf("content changed: len %d", len(got.Content))
	}
	vers, err := s.ListPostVersionsForTeamPost(ctx, team.ID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(vers) != 1 || vers[0].Content != short {
		t.Fatalf("versions: %#v", vers)
	}
}
