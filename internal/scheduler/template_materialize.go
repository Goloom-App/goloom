package scheduler

import (
	"context"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/scheduling"
)

func materializeHorizonEnd(now time.Time, horizonDays int) time.Time {
	if horizonDays <= 0 {
		return now.UTC()
	}
	return now.UTC().Add(time.Duration(horizonDays) * 24 * time.Hour)
}

func (s *Service) materializePostTemplates(ctx context.Context) error {
	templates, err := s.store.ListEnabledPostTemplates(ctx, 100)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, tmpl := range templates {
		if err := s.materializeTemplateHorizon(ctx, tmpl, now); err != nil {
			s.logger.Error("template materialize failed", "template_id", tmpl.ID, "error", err)
		}
	}
	return nil
}

func (s *Service) materializeTemplateHorizon(ctx context.Context, tmpl domain.PostTemplate, now time.Time) error {
	horizonEnd := materializeHorizonEnd(now, tmpl.MaterializeHorizonDays)
	for i := 0; i < 64; i++ {
		if tmpl.NextMaterializeAt == nil {
			return nil
		}
		occ := tmpl.NextMaterializeAt.UTC()
		if occ.After(horizonEnd) {
			return nil
		}
		advanced, err := s.materializeTemplateOccurrence(ctx, &tmpl, occ)
		if err != nil {
			return err
		}
		if !advanced {
			return nil
		}
		fresh, err := s.store.GetPostTemplate(ctx, tmpl.TeamID, tmpl.ID)
		if err != nil {
			return err
		}
		tmpl = fresh
	}
	s.logger.Warn("template materialize hit iteration cap", "template_id", tmpl.ID)
	return nil
}

func (s *Service) materializeTemplateOccurrence(ctx context.Context, tmpl *domain.PostTemplate, occ time.Time) (bool, error) {
	rule, err := scheduling.ParseRecurrenceJSON(tmpl.RecurrenceJSON)
	if err != nil {
		s.logger.Warn("invalid template recurrence_json", "template_id", tmpl.ID, "error", err)
		return false, nil
	}
	nextOcc, err := scheduling.NextOccurrence(rule, occ)
	if err != nil {
		s.logger.Warn("template next occurrence failed", "template_id", tmpl.ID, "error", err)
		return false, nil
	}

	skipped, err := s.store.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, occ)
	if err != nil {
		return false, err
	}
	if skipped {
		scheduledAt := s.maybeShiftOccurrence(ctx, tmpl, occ)
		if scheduledAt == nil {
			if err := s.store.AdvancePostTemplateSchedule(ctx, tmpl.ID, &nextOcc, tmpl.CounterNext); err != nil {
				return false, err
			}
			return true, nil
		}
		occ = scheduledAt.UTC()
	}

	mainExists, err := s.store.HasPostTemplateRoleMaterialized(ctx, tmpl.ID, occ, domain.TemplatePostRoleMain)
	if err != nil {
		return false, err
	}
	// Advance the counter only when we actually materialize a new round. When
	// the occurrence was already materialized (e.g. next_materialize_at was reset
	// backward by a template edit over horizon-materialized posts), we just step
	// past it: bumping the counter here would inflate it without creating any
	// post and drift it away from the announcement counter.
	createAt := occ
	nextCounter := tmpl.CounterNext
	if !mainExists {
		if err := s.createScheduledPostFromTemplate(ctx, tmpl, occ, occ, domain.TemplatePostRoleMain); err != nil {
			return false, nil
		}
		nextCounter = tmpl.CounterNext + 1
	}
	if err := s.store.AdvancePostTemplateSchedule(ctx, tmpl.ID, &nextOcc, nextCounter); err != nil {
		return false, err
	}
	if err := s.materializeAnnouncement(ctx, tmpl, createAt, !mainExists); err != nil {
		s.logger.Error("announcement materialize failed", "template_id", tmpl.ID, "error", err)
	}
	return true, nil
}
