package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/store"
)

var (
	ErrRegenerateBlocked = errors.New("regenerate blocked: posts already published or publishing")
	ErrRegenerateNoPosts = errors.New("regenerate: no posts in scope")
)

func (s *Service) RegeneratePostTemplateOccurrence(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) (domain.PostTemplateRegenerateResult, error) {
	return s.mutatePostTemplate(ctx, templateID, func(ctx context.Context) (domain.PostTemplateRegenerateResult, error) {
		return s.regeneratePostTemplateOccurrence(ctx, teamID, templateID, occurrenceAt)
	})
}

func (s *Service) regeneratePostTemplateOccurrence(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) (domain.PostTemplateRegenerateResult, error) {
	tmpl, err := s.store.GetPostTemplate(ctx, teamID, templateID)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	occ := occurrenceAt.UTC()
	linked, err := s.store.ListPostTemplateLinkedPosts(ctx, teamID, templateID)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	scope := filterLinkedPostsForOccurrence(linked, occ)
	if len(scope) == 0 {
		return domain.PostTemplateRegenerateResult{}, ErrRegenerateNoPosts
	}
	if blocked := blockedLinkedPosts(scope); len(blocked) > 0 {
		return domain.PostTemplateRegenerateResult{}, ErrRegenerateBlocked
	}
	regenerable := regenerableLinkedPosts(scope)
	if len(regenerable) == 0 {
		return domain.PostTemplateRegenerateResult{}, ErrRegenerateNoPosts
	}
	mainCounter, annCounter, ok := countersForOccurrence(scope, occ)
	if !ok {
		mainCounter = tmpl.CounterNext
		if mainCounter < 1 {
			mainCounter = 1
		}
		annCounter = tmpl.AnnouncementCounterNext
		if annCounter < 1 {
			annCounter = 1
		}
	}

	if err := s.cancelPendingRecurringAIJobs(ctx, teamID, templateID, &occ); err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	deleted, err := s.deleteLinkedPosts(ctx, teamID, templateID, regenerable)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	if err := s.rematerializeOccurrence(ctx, tmpl, occ, mainCounter, annCounter); err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	return domain.PostTemplateRegenerateResult{
		DeletedPosts:           deleted,
		RegeneratedOccurrences: 1,
	}, nil
}

func (s *Service) RegeneratePostTemplateHorizon(ctx context.Context, teamID, templateID string) (domain.PostTemplateRegenerateResult, error) {
	return s.mutatePostTemplate(ctx, templateID, func(ctx context.Context) (domain.PostTemplateRegenerateResult, error) {
		return s.regeneratePostTemplateHorizon(ctx, teamID, templateID)
	})
}

func (s *Service) regeneratePostTemplateHorizon(ctx context.Context, teamID, templateID string) (domain.PostTemplateRegenerateResult, error) {
	tmpl, err := s.store.GetPostTemplate(ctx, teamID, templateID)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	now := time.Now().UTC()
	horizonEnd := materializeHorizonEnd(now, tmpl.MaterializeHorizonDays)
	if tmpl.MaterializeHorizonDays <= 0 {
		horizonEnd = now.Add(366 * 24 * time.Hour)
	}

	linked, err := s.store.ListPostTemplateLinkedPosts(ctx, teamID, templateID)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	scope := filterLinkedPostsForHorizon(linked, now, horizonEnd)
	if len(scope) == 0 {
		return domain.PostTemplateRegenerateResult{}, ErrRegenerateNoPosts
	}
	if blocked := blockedLinkedPosts(scope); len(blocked) > 0 {
		return domain.PostTemplateRegenerateResult{}, ErrRegenerateBlocked
	}
	regenerable := regenerableLinkedPosts(scope)
	if len(regenerable) == 0 {
		return domain.PostTemplateRegenerateResult{}, ErrRegenerateNoPosts
	}

	occurrences := uniqueOccurrences(regenerable)
	if len(occurrences) == 0 {
		return domain.PostTemplateRegenerateResult{}, ErrRegenerateNoPosts
	}
	earliest := occurrences[0]
	mainCounter, annCounter, ok := countersForOccurrence(scope, earliest)
	if !ok {
		return domain.PostTemplateRegenerateResult{}, fmt.Errorf("regenerate horizon: missing counters for %s", earliest.Format(time.RFC3339))
	}

	if err := s.cancelPendingRecurringAIJobs(ctx, teamID, templateID, nil); err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	deleted, err := s.deleteLinkedPosts(ctx, teamID, templateID, regenerable)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	if err := s.store.SetPostTemplateMaterializationState(ctx, templateID, &earliest, mainCounter, annCounter); err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	fresh, err := s.store.GetPostTemplate(ctx, teamID, templateID)
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	if err := s.materializeTemplateHorizon(ctx, fresh, now); err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	return domain.PostTemplateRegenerateResult{
		DeletedPosts:           deleted,
		RegeneratedOccurrences: len(occurrences),
	}, nil
}

func (s *Service) rematerializeOccurrence(ctx context.Context, tmpl domain.PostTemplate, occ time.Time, mainCounter, annCounter int) error {
	tmpl.CounterNext = mainCounter
	tmpl.AnnouncementCounterNext = annCounter
	createAt := occ.UTC()
	if shift := s.maybeShiftOccurrence(ctx, &tmpl, occ); shift != nil {
		createAt = shift.UTC()
	}
	if err := s.createScheduledPostFromTemplate(ctx, &tmpl, createAt, occ, domain.TemplatePostRoleMain); err != nil {
		return err
	}
	return s.materializeAnnouncement(ctx, &tmpl, createAt, false)
}

func (s *Service) deleteLinkedPosts(ctx context.Context, teamID, templateID string, posts []domain.PostTemplateLinkedPost) (int, error) {
	ids := make([]string, 0, len(posts))
	for _, p := range posts {
		ids = append(ids, p.ID)
	}
	return s.store.DeletePostTemplateLinkedPosts(ctx, teamID, templateID, ids)
}

func filterLinkedPostsForOccurrence(posts []domain.PostTemplateLinkedPost, occ time.Time) []domain.PostTemplateLinkedPost {
	out := make([]domain.PostTemplateLinkedPost, 0, len(posts))
	for _, p := range posts {
		if p.TemplateOccurrenceAt.Equal(occ) {
			out = append(out, p)
		}
	}
	return out
}

func filterLinkedPostsForHorizon(posts []domain.PostTemplateLinkedPost, horizonStart, horizonEnd time.Time) []domain.PostTemplateLinkedPost {
	out := make([]domain.PostTemplateLinkedPost, 0, len(posts))
	for _, p := range posts {
		if !p.TemplateOccurrenceAt.Before(horizonStart) && !p.TemplateOccurrenceAt.After(horizonEnd) {
			out = append(out, p)
		}
	}
	return out
}

func regenerableLinkedPosts(posts []domain.PostTemplateLinkedPost) []domain.PostTemplateLinkedPost {
	out := make([]domain.PostTemplateLinkedPost, 0, len(posts))
	for _, p := range posts {
		if isRegenerableStatus(p.Status) {
			out = append(out, p)
		}
	}
	return out
}

func blockedLinkedPosts(posts []domain.PostTemplateLinkedPost) []domain.PostTemplateLinkedPost {
	out := make([]domain.PostTemplateLinkedPost, 0)
	for _, p := range posts {
		if isBlockedRegenerateStatus(p.Status) {
			out = append(out, p)
		}
	}
	return out
}

func isRegenerableStatus(status domain.PostStatus) bool {
	switch status {
	case domain.PostStatusDraft, domain.PostStatusPending, domain.PostStatusFailed:
		return true
	default:
		return false
	}
}

func isBlockedRegenerateStatus(status domain.PostStatus) bool {
	return status == domain.PostStatusPosted || status == domain.PostStatusProcessing
}

func countersForOccurrence(posts []domain.PostTemplateLinkedPost, occ time.Time) (mainCounter, annCounter int, ok bool) {
	mainCounter = -1
	annCounter = -1
	for _, p := range posts {
		if !p.TemplateOccurrenceAt.Equal(occ) {
			continue
		}
		ctr := 1
		if p.TemplateCounter != nil && *p.TemplateCounter >= 1 {
			ctr = *p.TemplateCounter
		}
		switch p.TemplatePostRole {
		case domain.TemplatePostRoleMain:
			mainCounter = ctr
		case domain.TemplatePostRoleAnnouncement:
			annCounter = ctr
		}
	}
	if mainCounter < 1 && annCounter < 1 {
		return 0, 0, false
	}
	if mainCounter < 1 {
		mainCounter = 1
	}
	if annCounter < 1 {
		annCounter = 1
	}
	return mainCounter, annCounter, true
}

func uniqueOccurrences(posts []domain.PostTemplateLinkedPost) []time.Time {
	seen := make(map[int64]struct{})
	var out []time.Time
	for _, p := range posts {
		key := p.TemplateOccurrenceAt.UTC().UnixNano()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p.TemplateOccurrenceAt.UTC())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Before(out[j])
	})
	return out
}

func (s *Service) cancelPendingRecurringAIJobs(ctx context.Context, teamID, templateID string, occurrenceAt *time.Time) error {
	return s.cancelPendingRecurringAIJobsMatching(ctx, teamID, templateID, func(occ *time.Time) bool {
		if occurrenceAt == nil {
			return true
		}
		return occ != nil && occ.Equal(occurrenceAt.UTC())
	})
}

// cancelPendingRecurringAIJobsFrom cancels in-flight enhancement for every round
// at or after from, matching the tear-down scope of a skip or shift.
func (s *Service) cancelPendingRecurringAIJobsFrom(ctx context.Context, teamID, templateID string, from time.Time) error {
	return s.cancelPendingRecurringAIJobsMatching(ctx, teamID, templateID, func(occ *time.Time) bool {
		return occ != nil && !occ.Before(from.UTC())
	})
}

// Lock the source before snapshots so cancellation and callback completion use
// the same source -> job lock order in both backends.
func (s *Service) mutatePostTemplate(ctx context.Context, templateID string, mutate func(context.Context) (domain.PostTemplateRegenerateResult, error)) (domain.PostTemplateRegenerateResult, error) {
	var result domain.PostTemplateRegenerateResult
	err := s.store.WithAuditTransaction(ctx, func(ctx context.Context) error {
		if err := s.store.LockAutomationSource(ctx, "post_template", templateID); err != nil {
			return err
		}
		var err error
		result, err = mutate(ctx)
		return err
	})
	if err != nil {
		return domain.PostTemplateRegenerateResult{}, err
	}
	return result, nil
}

func (s *Service) pendingRecurringAIJobs(ctx context.Context, teamID, templateID string, match func(*time.Time) bool) ([]domain.AIJob, error) {
	// Team history may exceed the UI's 200-row window; cancellation must inspect
	// every potentially active source job rather than silently skipping old jobs.
	jobs, err := s.store.ListAIJobs(ctx, teamID, math.MaxInt32)
	if err != nil {
		return nil, err
	}
	selected := make([]domain.AIJob, 0)
	for _, job := range jobs {
		if job.TeamID != teamID || job.Origin != domain.AIJobOriginAutomation || job.Type != domain.AIJobTypeVoiceEngine || (job.Status != domain.AIJobStatusPending && job.Status != domain.AIJobStatusProcessing) {
			continue
		}
		meta := parseRecurringAutomationMetaFromPayload(job.Payload)
		if meta != nil && meta.TemplateID == templateID && match(meta.TemplateOccurrenceAt) {
			selected = append(selected, job)
		}
	}
	return selected, nil
}

func (s *Service) cancelPendingRecurringAIJobsMatching(ctx context.Context, teamID, templateID string, match func(occurrenceAt *time.Time) bool) error {
	jobs, err := s.pendingRecurringAIJobs(ctx, teamID, templateID, match)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		claimed, err := s.store.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusFailed, nil, "cancelled")
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		meta := parseRecurringAutomationMetaFromPayload(job.Payload)
		metadata := map[string]string{"job_id": job.ID, "template_id": templateID, "post_kind": meta.PostKind, "author_user_id": job.AuthorUserID, "reason": "recurring_change"}
		if meta.TemplateOccurrenceAt != nil {
			metadata["occurrence_at"] = meta.TemplateOccurrenceAt.UTC().Format(time.RFC3339Nano)
		}
		if p := auth.PrincipalFromContext(ctx); p != nil {
			metadata["initiator_user_id"] = p.User.ID
			if p.Kind == domain.AuditActorToken && p.TokenID != nil {
				metadata["initiator_token_id"] = *p.TokenID
			}
		}
		if err := s.store.InsertAuditEvent(ctx, automationEvent(teamID, "ai_job.cancel", "ai_job", job.ID, metadata)); err != nil {
			return err
		}
		if s.jobManager != nil {
			store.AfterAuditCommit(ctx, func(context.Context) { s.jobManager.Cancel(job.ID) })
		}
	}
	return nil
}

type recurringAutomationMetaLite struct {
	PostKind             string
	TemplateCounter      int
	TemplateID           string
	TemplateOccurrenceAt *time.Time
}

func parseRecurringAutomationMetaFromPayload(payload []byte) *recurringAutomationMetaLite {
	var outer struct {
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(payload, &outer); err != nil {
		return nil
	}
	var params struct {
		RecurringAutomation struct {
			TemplateID           string `json:"template_id"`
			PostKind             string `json:"post_kind"`
			TemplateCounter      int    `json:"template_counter"`
			TemplateOccurrenceAt string `json:"template_occurrence_at"`
		} `json:"recurring_automation"`
	}
	raw := payload
	if len(outer.Params) > 0 {
		raw = outer.Params
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil
	}
	if strings.TrimSpace(params.RecurringAutomation.TemplateID) == "" {
		return nil
	}
	meta := &recurringAutomationMetaLite{
		TemplateID:      params.RecurringAutomation.TemplateID,
		PostKind:        strings.TrimSpace(params.RecurringAutomation.PostKind),
		TemplateCounter: params.RecurringAutomation.TemplateCounter,
	}
	if meta.PostKind == "" {
		meta.PostKind = domain.TemplatePostRoleMain
	}
	if ts := strings.TrimSpace(params.RecurringAutomation.TemplateOccurrenceAt); ts != "" {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			u := parsed.UTC()
			meta.TemplateOccurrenceAt = &u
		}
	}
	return meta
}
