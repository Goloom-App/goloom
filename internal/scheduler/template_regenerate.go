package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

var (
	ErrRegenerateBlocked = errors.New("regenerate blocked: posts already published or publishing")
	ErrRegenerateNoPosts = errors.New("regenerate: no posts in scope")
)

func (s *Service) RegeneratePostTemplateOccurrence(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) (domain.PostTemplateRegenerateResult, error) {
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
	jobs, err := s.store.ListAIJobs(ctx, teamID, 200)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.Status != domain.AIJobStatusPending && job.Status != domain.AIJobStatusProcessing {
			continue
		}
		meta := parseRecurringAutomationMetaFromPayload(job.Payload)
		if meta == nil || meta.TemplateID != templateID {
			continue
		}
		if occurrenceAt != nil {
			if meta.TemplateOccurrenceAt == nil || !meta.TemplateOccurrenceAt.Equal(occurrenceAt.UTC()) {
				continue
			}
		}
		if err := s.store.UpdateAIJobStatus(ctx, job.ID, domain.AIJobStatusFailed, nil, "cancelled"); err != nil {
			return err
		}
	}
	return nil
}

type recurringAutomationMetaLite struct {
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
		TemplateID: params.RecurringAutomation.TemplateID,
	}
	if ts := strings.TrimSpace(params.RecurringAutomation.TemplateOccurrenceAt); ts != "" {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			u := parsed.UTC()
			meta.TemplateOccurrenceAt = &u
		}
	}
	return meta
}
