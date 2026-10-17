package scheduler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

// sourceKey correlates RSS items without retaining credential-bearing URLs.
func sourceKey(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func automationEvent(teamID, action, targetType, targetID string, metadata map[string]string) domain.AuditEvent {
	if metadata == nil {
		metadata = make(map[string]string)
	}
	metadata["channel"] = "scheduler"
	if metadata["outcome"] == "" {
		metadata["outcome"] = "success"
	}
	return domain.AuditEvent{ID: uuid.NewString(), TeamID: teamID, ActorKind: domain.AuditActorSystem, ActorName: "Automation", Action: action, TargetType: targetType, TargetID: &targetID, Metadata: metadata}
}

func publicationEvent(post domain.ScheduledPost, accountID, attemptID, action, outcome, code string) domain.AuditEvent {
	return automationEvent(post.TeamID, action, "post", post.ID, map[string]string{"post_id": post.ID, "account_id": accountID, "attempt_id": attemptID, "attempt": strconv.Itoa(post.AttemptCount + 1), "outcome": outcome, "error_code": code})
}

// persistTargetOutcome saves observed provider results and a durable pending
// event together. Audit delivery never invokes the external provider again.
func (s *Service) persistTargetOutcome(ctx context.Context, post domain.ScheduledPost, accountID, attemptID string, status domain.PostStatus, publishedURL, lastError string, metadata map[string]string, remoteID, action, outcome, code string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err := s.store.WithAuditTransaction(ctx, func(ctx context.Context) error {
		if err := s.store.MarkPostTargetResult(ctx, post.ID, accountID, status, publishedURL, lastError, metadata, remoteID); err != nil {
			return err
		}
		return s.store.EnqueueAuditEvent(ctx, publicationEvent(post, accountID, attemptID, action, outcome, code))
	})
	if err != nil {
		s.logger.Error("publication outcome persistence failed; reconciliation required", "post_id", post.ID, "account_id", accountID, "error", err)
		return err
	}
	if err := s.store.DrainAuditEvents(ctx); err != nil {
		s.logger.Error("audit delivery pending", "post_id", post.ID, "account_id", accountID, "error", err)
	}
	return nil
}

func (s *Service) createAutomationPost(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput, metadata map[string]string, after func(context.Context, domain.ScheduledPost) error) (domain.ScheduledPost, error) {
	var post domain.ScheduledPost
	err := s.store.WithAuditTransaction(ctx, func(ctx context.Context) error {
		if input.PostTemplateID != nil && input.TemplateOccurrenceAt != nil {
			if err := s.store.LockAutomationSource(ctx, "post_template", *input.PostTemplateID); err != nil {
				return err
			}
			exists, err := s.store.HasPostTemplateRoleMaterialized(ctx, *input.PostTemplateID, *input.TemplateOccurrenceAt, input.TemplatePostRole)
			if err != nil || exists {
				return err
			}
		}
		var err error
		post, err = s.store.CreateScheduledPost(ctx, teamID, principal, input)
		if err != nil {
			return err
		}
		if after != nil {
			if err := after(ctx, post); err != nil {
				return err
			}
		}
		metadata["post_id"] = post.ID
		if input.AuthorUserID != nil {
			metadata["author_user_id"] = *input.AuthorUserID
		}
		if p := auth.PrincipalFromContext(ctx); p != nil {
			metadata["initiator_user_id"] = p.User.ID
			if p.TokenID != nil && p.Kind == domain.AuditActorToken {
				metadata["initiator_token_id"] = *p.TokenID
			}
		}
		return s.store.InsertAuditEvent(ctx, automationEvent(teamID, "post.automation_create", "post", post.ID, metadata))
	})
	return post, err
}
