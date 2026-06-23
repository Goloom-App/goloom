package api

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestPatchPost_ValidateMerged_ScheduledAtOnlyUsesStoredOverride(t *testing.T) {
	s := newValidationMemoryStore(t)
	api := newTestAPI(t, s)
	teamID, bsID, mastoID := seedCrossPostAccounts(t, s)

	ctx := context.Background()
	u, _ := s.UpsertOIDCUser(ctx, "patch-api", "p@x", "P")
	principal := domain.AuthenticatedPrincipal{User: u}
	when := time.Now().UTC().Add(24 * time.Hour)
	long := string(runeLen(419))
	short := string(runeLen(232))

	post, err := s.CreateScheduledPost(ctx, teamID, principal, domain.CreatePostInput{
		Title: "Test post", Content: long, ScheduledAt: when, TargetAccounts: []string{bsID, mastoID},
		AccountContentOverride: map[string]string{bsID: short},
	})
	if err != nil {
		t.Fatal(err)
	}

	existing, err := s.GetScheduledPost(ctx, teamID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := s.ListPostVersionsForTeamPost(ctx, teamID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	patch := domain.UpdatePostPatch{
		ScheduledAt: domain.PatchField[time.Time]{Set: true, Value: when.Add(2 * time.Hour)},
	}
	merged, _ := domain.ApplyPostPatch(existing, versions, patch)
	resp, _, err := api.validatePostInput(ctx, teamID, merged)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Valid {
		t.Fatalf("valid=false, destinations: %+v", resp.Destinations)
	}
	bs := findDest(resp.Destinations, bsID)
	if bs == nil || !bs.Valid || bs.Length != 232 {
		t.Fatalf("bluesky: %+v", bs)
	}
}

func TestPatchPost_Store_ScheduledAtOnlyPreservesVersions(t *testing.T) {
	s := newValidationMemoryStore(t)
	teamID, bsID, _ := seedCrossPostAccounts(t, s)
	ctx := context.Background()
	u, _ := s.UpsertOIDCUser(ctx, "patch-store", "ps@x", "PS")
	principal := domain.AuthenticatedPrincipal{User: u}
	when := time.Now().UTC().Add(24 * time.Hour)
	long := string(runeLen(419))
	short := string(runeLen(232))

	post, err := s.CreateScheduledPost(ctx, teamID, principal, domain.CreatePostInput{
		Title: "Test post", Content: long, ScheduledAt: when, TargetAccounts: []string{bsID},
		AccountContentOverride: map[string]string{bsID: short},
	})
	if err != nil {
		t.Fatal(err)
	}

	newWhen := when.Add(3 * time.Hour)
	if _, err := s.PatchScheduledPost(ctx, teamID, post.ID, domain.UpdatePostPatch{
		ScheduledAt: domain.PatchField[time.Time]{Set: true, Value: newWhen},
	}); err != nil {
		t.Fatal(err)
	}
	vers, err := s.ListPostVersionsForTeamPost(ctx, teamID, post.ID)
	if err != nil || len(vers) != 1 || vers[0].Content != short {
		t.Fatalf("versions: %#v err=%v", vers, err)
	}
}
