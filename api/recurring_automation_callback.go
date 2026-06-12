package api

import (
	"encoding/json"
	"context"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) finishRecurringAutomationFromAI(ctx context.Context, job domain.AIJob, rawResult json.RawMessage, meta *recurringAutomationMeta) {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		_ = json.Unmarshal(rawResult, &res)
	}
	content := strings.TrimSpace(res.Content)
	if content == "" {
		a.log.WarnContext(ctx, "recurring automation: empty ai content, using template fallback",
			"job_id", job.ID, "template_id", meta.TemplateID, "post_kind", meta.PostKind)
		a.finishRecurringAutomationFallback(ctx, job, meta)
		return
	}
	a.createRecurringAutomationPost(ctx, job, meta, content, res, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) finishRecurringAutomationFallback(ctx context.Context, job domain.AIJob, meta *recurringAutomationMeta) {
	content := meta.FallbackContent
	if content == "" {
		a.log.WarnContext(ctx, "recurring automation: fallback skipped, no template content",
			"job_id", job.ID, "template_id", meta.TemplateID)
		return
	}
	a.log.WarnContext(ctx, "recurring automation: using expanded template fallback",
		"job_id", job.ID, "template_id", meta.TemplateID, "post_kind", meta.PostKind)
	a.createRecurringAutomationPost(ctx, job, meta, content, aiCallbackResult{}, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) createRecurringAutomationPost(
	ctx context.Context,
	job domain.AIJob,
	meta *recurringAutomationMeta,
	content string,
	res aiCallbackResult,
	targetAccounts []string,
) {
	if len(targetAccounts) == 0 {
		return
	}
	normalizedOverrides := domain.NormalizeAccountContentOverride(res.AccountContentOverride, targetAccounts)
	scheduledAt := meta.ScheduledAt
	if res.ScheduledAt != nil && !res.ScheduledAt.IsZero() {
		scheduledAt = res.ScheduledAt.UTC()
	} else if res.SuggestedScheduledAt != nil && !res.SuggestedScheduledAt.IsZero() {
		scheduledAt = res.SuggestedScheduledAt.UTC()
	}
	draft := recurringAutomationDraftFromMeta(meta)
	if meta.PostKind != "announcement" && meta.OutputMode == domain.AutomationOutputPublishNow {
		draft = false
		scheduledAt = time.Now().UTC()
	}

	tplID := meta.TemplateID
	counterVal := meta.TemplateCounter
	role := domain.TemplatePostRoleMain
	if meta.PostKind == domain.TemplatePostRoleAnnouncement {
		role = domain.TemplatePostRoleAnnouncement
	}
	principal := domain.AuthenticatedPrincipal{
		User: domain.User{ID: job.AuthorUserID},
		Kind: "api_token",
	}
	_, err := a.store.CreateScheduledPost(ctx, job.TeamID, principal, domain.CreatePostInput{
		Title:                  domain.ResolveAutomationPostTitle(meta.PostTitle, res.Title),
		Content:                content,
		TargetAccounts:         targetAccounts,
		AccountContentOverride: normalizedOverrides,
		ScheduledAt:            scheduledAt,
		Draft:                  draft,
		AuthorUserID:           &job.AuthorUserID,
		Source:                 domain.PostSourceAutomation,
		PostTemplateID:         &tplID,
		TemplateCounter:        &counterVal,
		TemplateOccurrenceAt:   meta.TemplateOccurrenceAt,
		TemplatePostRole:       role,
		UseVersions:            len(normalizedOverrides) > 0,
	})
	if err != nil {
		a.log.ErrorContext(ctx, "recurring automation: create scheduled post failed",
			"job_id", job.ID, "template_id", meta.TemplateID, "post_kind", meta.PostKind, "error", err)
		return
	}
}
