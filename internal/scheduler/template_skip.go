package scheduler

import (
	"context"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// SkipPostTemplateOccurrence records a skip (shiftTo == nil) or a shift for occ
// and, when that round was already materialized, tears the materialized posts
// down and re-walks the horizon from occ.
//
// Recording the skip row alone is not enough. IsPostTemplateOccurrenceSkipped is
// only consulted while the materialization cursor walks over an occurrence for
// the first time, so a round the cursor already passed would keep its posts —
// the announcement stays in the calendar and nothing is renumbered.
//
// Everything from occ onwards is torn down, not just occ itself: dropping a slot
// shifts every later edition number down by one, so those posts have to be
// rewritten too. The cursor is then rewound to occ carrying the counters that
// round owned, which lets the re-walk step over the skipped slot without burning
// the edition number and hand it to the next round instead.
func (s *Service) SkipPostTemplateOccurrence(ctx context.Context, teamID, templateID string, occurrenceAt time.Time, shiftTo *time.Time) (domain.PostTemplateRegenerateResult, error) {
	tmpl, err := s.store.GetPostTemplate(ctx, teamID, templateID)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	occ := occurrenceAt.UTC()

	linked, err := s.store.ListPostTemplateLinkedPosts(ctx, teamID, templateID)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	scope := filterLinkedPostsFromOccurrence(linked, occ)
	if blocked := blockedLinkedPosts(scope); len(blocked) > 0 {
		return domain.PostTemplateRegenerateResult{}, ErrRegenerateBlocked
	}

	// Record the skip first so the re-walk below already sees it.
	if shiftTo != nil {
		err = s.store.ShiftPostTemplateOccurrence(ctx, teamID, templateID, occ, shiftTo.UTC())
	} else {
		err = s.store.AddPostTemplateSkip(ctx, teamID, templateID, occ)
	}
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}

	regenerable := regenerableLinkedPosts(scope)
	if len(regenerable) == 0 {
		// The cursor has not reached occ yet: nothing is materialized, so the
		// skip row on its own does the job when the walk gets there.
		return domain.PostTemplateRegenerateResult{}, nil
	}

	mainCounter, annCounter, ok := countersForOccurrence(scope, occ)
	if !ok {
		mainCounter = tmpl.CounterNext
		annCounter = tmpl.AnnouncementCounterNext
	}
	if mainCounter < 1 {
		mainCounter = 1
	}
	if annCounter < 1 {
		annCounter = 1
	}

	if err := s.cancelPendingRecurringAIJobsFrom(ctx, teamID, templateID, occ); err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	deleted, err := s.deleteLinkedPosts(ctx, teamID, templateID, regenerable)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	if err := s.store.SetPostTemplateMaterializationState(ctx, templateID, &occ, mainCounter, annCounter); err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}

	fresh, err := s.store.GetPostTemplate(ctx, teamID, templateID)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	if err := s.materializeTemplateHorizon(ctx, fresh, time.Now().UTC()); err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	return domain.PostTemplateRegenerateResult{
		DeletedPosts:           deleted,
		RegeneratedOccurrences: len(uniqueOccurrences(regenerable)),
	}, nil
}

// filterLinkedPostsFromOccurrence returns every linked post at or after occ.
// There is deliberately no upper bound: a dropped slot renumbers all later
// rounds, including any left over beyond a since-shrunk horizon.
func filterLinkedPostsFromOccurrence(posts []domain.PostTemplateLinkedPost, occ time.Time) []domain.PostTemplateLinkedPost {
	out := make([]domain.PostTemplateLinkedPost, 0, len(posts))
	for _, p := range posts {
		if !p.TemplateOccurrenceAt.UTC().Before(occ) {
			out = append(out, p)
		}
	}
	return out
}
