package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

// TestPostgres_PatchScheduledPost_PublishNowAtomicVsConcurrentEdit reproduces the
// stale publish_now race: a publish_now (ScheduledAt + Draft=false) must only
// transition scheduled_at/status and must never replay a content snapshot, or a
// concurrent edit committed between the handler's read and write is lost.
//
// While edit N-1 is settled, a burst of publish_now calls starts and snapshots
// content N-1; the final edit commits content N while those publishes are still
// in flight. Pre-fix the trailing publish_now replays content N-1 over N.
func TestPostgres_PatchScheduledPost_PublishNowAtomicVsConcurrentEdit(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "pna-"+uuid.NewString(), "pna@pg.test", "Pna")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "pna-" + uuid.NewString(), Description: ""})
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "bluesky", AuthType: domain.AccountAuthTypeAppPassword,
		InstanceURL: "https://b.social", Username: "b", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}

	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content: "original", ScheduledAt: time.Now().UTC().Add(2 * time.Hour),
		TargetAccounts: []string{acc.ID}, Draft: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	publishNow := func() error {
		_, err := s.PatchScheduledPost(ctx, team.ID, post.ID, domain.UpdatePostPatch{
			ScheduledAt: domain.PatchField[time.Time]{Set: true, Value: time.Now().UTC().Add(30 * time.Second)},
			Draft:       domain.PatchField[bool]{Set: true, Value: false},
		})
		return err
	}

	const edits = 20
	const burst = 24
	for i := 1; i <= edits; i++ {
		val := fmt.Sprintf("edit-%04d", i)

		var wg sync.WaitGroup
		for b := 0; b < burst; b++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := publishNow(); err != nil {
					t.Errorf("publish_now: %v", err)
				}
			}()
		}
		time.Sleep(200 * time.Microsecond)

		if _, err := s.PatchScheduledPost(ctx, team.ID, post.ID, domain.UpdatePostPatch{
			Content: domain.PatchField[string]{Set: true, Value: val},
		}); err != nil {
			t.Fatalf("edit %d: %v", i, err)
		}
		wg.Wait()
	}

	got, err := s.GetScheduledPost(ctx, team.ID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("edit-%04d", edits)
	if got.Content != want {
		t.Fatalf("content = %q, want %q: publish_now replayed a stale snapshot over a concurrently edited draft", got.Content, want)
	}
	if got.Status != domain.PostStatusPending {
		t.Fatalf("status = %q, want %q", got.Status, domain.PostStatusPending)
	}
}

// TestPostgres_PatchScheduledPost_ReviewTransitionMovesDraftAtomically is the
// positive path for the explicit queue review-transition signal: a draft moves
// to pending at the patched time and stored content survives untouched.
func TestPostgres_PatchScheduledPost_ReviewTransitionMovesDraftAtomically(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "revp1-"+uuid.NewString(), "revp1@pg.test", "Revp1")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "revp-" + uuid.NewString(), Description: ""})
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "bluesky", AuthType: domain.AccountAuthTypeAppPassword,
		InstanceURL: "https://b.social", Username: "b", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}

	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Title: "Review item", Content: "draft body",
		ScheduledAt:    time.Now().UTC().Add(2 * time.Hour),
		TargetAccounts: []string{acc.ID}, Draft: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	at := time.Now().UTC().Add(30 * time.Second)
	got, err := s.PatchScheduledPost(ctx, team.ID, post.ID, domain.UpdatePostPatch{
		ScheduledAt:    domain.PatchField[time.Time]{Set: true, Value: at},
		Draft:          domain.PatchField[bool]{Set: true, Value: false},
		ReviewComplete: domain.PatchField[bool]{Set: true, Value: true},
	})
	if err != nil {
		t.Fatalf("review transition: %v", err)
	}
	if got.Status != domain.PostStatusPending {
		t.Fatalf("status = %q, want pending", got.Status)
	}
	if got.Content != "draft body" || got.Title != "Review item" {
		t.Fatalf("content/title replayed, got %q / %q", got.Content, got.Title)
	}
	if d := time.Until(got.ScheduledAt); d < 20*time.Second || d > 40*time.Second {
		t.Fatalf("scheduled_at = %v, want ~30s out (stale write would overwrite)", got.ScheduledAt)
	}
}

// TestPostgres_PatchScheduledPost_StaleReviewTransitionConflicts reproduces the
// race the status-guarded predicate closes: a composer save schedules the draft
// into a future pending state, then a stale queue review-transition arrives for
// the same post. The transition must fail with a conflict and leave the newer
// state (future timestamp + edited content) untouched.
func TestPostgres_PatchScheduledPost_StaleReviewTransitionConflicts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "revp2-"+uuid.NewString(), "revp2@pg.test", "Revp2")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "revp-" + uuid.NewString(), Description: ""})
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "bluesky", AuthType: domain.AccountAuthTypeAppPassword,
		InstanceURL: "https://b.social", Username: "b", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}

	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Title: "Review item", Content: "original",
		ScheduledAt:    time.Now().UTC().Add(2 * time.Hour),
		TargetAccounts: []string{acc.ID}, Draft: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Composer save wins first: full edit scheduling the draft 2h out.
	future := time.Now().UTC().Add(2 * time.Hour)
	if _, err := s.PatchScheduledPost(ctx, team.ID, post.ID, domain.UpdatePostPatch{
		Content:     domain.PatchField[string]{Set: true, Value: "edited by composer"},
		ScheduledAt: domain.PatchField[time.Time]{Set: true, Value: future},
		Draft:       domain.PatchField[bool]{Set: true, Value: false},
	}); err != nil {
		t.Fatalf("composer save: %v", err)
	}

	// Stale queue publish-now arrives after the post left draft.
	_, err = s.PatchScheduledPost(ctx, team.ID, post.ID, domain.UpdatePostPatch{
		ScheduledAt:    domain.PatchField[time.Time]{Set: true, Value: time.Now().UTC().Add(30 * time.Second)},
		Draft:          domain.PatchField[bool]{Set: true, Value: false},
		ReviewComplete: domain.PatchField[bool]{Set: true, Value: true},
	})
	if !errors.Is(err, domain.ErrReviewCompleteConflict) {
		t.Fatalf("stale review transition err = %v, want ErrReviewCompleteConflict", err)
	}

	got, err := s.GetScheduledPost(ctx, team.ID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.PostStatusPending {
		t.Fatalf("status = %q, want pending (unchanged)", got.Status)
	}
	if got.Content != "edited by composer" {
		t.Fatalf("content = %q, want composer edit preserved (stale action must not overwrite)", got.Content)
	}
	if d := time.Until(got.ScheduledAt); d < 1*time.Hour {
		t.Fatalf("scheduled_at = %v, want the composer's future timestamp, not ~now", got.ScheduledAt)
	}
}
