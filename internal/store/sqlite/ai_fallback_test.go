package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestPostFallbackReasonRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := makeAITeam(t, s)
	reason := "provider_failure"
	jobID := "01234567-89ab-cdef-0123-456789abcdef"
	for _, marker := range []*string{nil, &reason} {
		post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: u}, domain.CreatePostInput{Content: "Template fallback", ScheduledAt: time.Now().UTC().Add(time.Hour), Draft: true, Source: domain.PostSourceAutomation, AIFallbackReason: marker, AIJobID: &jobID})
		if err != nil {
			t.Fatal(err)
		}
		check := func(p domain.ScheduledPost) {
			t.Helper()
			if p.AIJobID == nil || *p.AIJobID != jobID {
				t.Fatalf("job ID=%v want %s", p.AIJobID, jobID)
			}
			if (p.AIFallbackReason == nil) != (marker == nil) || marker != nil && *p.AIFallbackReason != *marker {
				t.Fatalf("fallback reason=%v want %v", p.AIFallbackReason, marker)
			}
		}
		check(post)
		got, err := s.GetScheduledPost(ctx, team.ID, post.ID)
		if err != nil {
			t.Fatal(err)
		}
		check(got)
		posts, err := s.ListTeamPosts(ctx, team.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range posts {
			if p.ID == post.ID {
				check(p)
			}
		}
		review, err := s.ListAutomationReviewDrafts(ctx, team.ID, 20)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range review {
			if p.ID == post.ID {
				check(p.ScheduledPost)
			}
		}
	}
}
