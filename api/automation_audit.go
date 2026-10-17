package api

import (
	"context"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func jobAuditEvent(job domain.AIJob, action, targetType, targetID string, metadata map[string]string) domain.AuditEvent {
	if metadata == nil {
		metadata = make(map[string]string)
	}
	metadata["channel"], metadata["job_id"], metadata["initiator_user_id"] = "ai", job.ID, job.AuthorUserID
	if metadata["outcome"] == "" {
		metadata["outcome"] = "success"
	}
	return domain.AuditEvent{ID: uuid.NewString(), TeamID: job.TeamID, ActorKind: "system", ActorName: "Automation", Action: action, TargetType: targetType, TargetID: &targetID, Metadata: metadata}
}

func (a *API) createJobAutomationPost(ctx context.Context, job domain.AIJob, input domain.CreatePostInput, metadata map[string]string) (domain.ScheduledPost, error) {
	post, err := a.store.CreateScheduledPost(ctx, job.TeamID, domain.AuthenticatedPrincipal{User: domain.User{ID: job.AuthorUserID}, Kind: "system"}, input)
	if err != nil {
		return domain.ScheduledPost{}, err
	}
	metadata["post_id"] = post.ID
	if err := a.store.InsertAuditEvent(ctx, jobAuditEvent(job, "post.automation_create", "post", post.ID, metadata)); err != nil {
		return domain.ScheduledPost{}, err
	}
	return post, nil
}
