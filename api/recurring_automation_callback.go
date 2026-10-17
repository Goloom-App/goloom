package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postservice"
)

func (a *API) finishRecurringAutomationFromAI(ctx context.Context, job domain.AIJob, rawResult json.RawMessage, meta *recurringAutomationMeta) error {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		if err := json.Unmarshal(rawResult, &res); err != nil {
			if fallbackErr := a.finishRecurringAutomationFallback(ctx, job, meta); fallbackErr != nil {
				return fmt.Errorf("decode automation result: %w; fallback failed: %w", err, fallbackErr)
			}
			return fmt.Errorf("decode automation result: %w; %w", err, errAutomationTemplateFallback)
		}
	}
	content := strings.TrimSpace(res.Content)
	if content == "" {
		a.log.WarnContext(ctx, "recurring automation: empty ai content, using template fallback",
			"job_id", job.ID, "template_id", meta.TemplateID, "post_kind", meta.PostKind)
		if err := a.finishRecurringAutomationFallback(ctx, job, meta); err != nil {
			return err
		}
		return fmt.Errorf("empty AI content; %w", errAutomationTemplateFallback)
	}
	err := a.createRecurringAutomationPost(ctx, job, meta, content, res, targetAccountIDsFromJobPayload(job.Payload))
	if !errors.Is(err, errAutomationPostValidation) {
		return err
	}
	a.log.WarnContext(ctx, "recurring automation: invalid AI content, trying template fallback", "job_id", job.ID, "error", err)
	if fallbackErr := a.finishRecurringAutomationFallback(ctx, job, meta); fallbackErr != nil {
		return fmt.Errorf("AI post validation: %w; fallback failed: %w", err, fallbackErr)
	}
	return fmt.Errorf("AI post validation: %w; %w", err, errAutomationTemplateFallback)
}

func (a *API) finishRecurringAutomationFallback(ctx context.Context, job domain.AIJob, meta *recurringAutomationMeta) error {
	content := meta.FallbackContent
	if content == "" {
		a.log.WarnContext(ctx, "recurring automation: fallback skipped, no template content",
			"job_id", job.ID, "template_id", meta.TemplateID)
		return fmt.Errorf("automation fallback content is empty")
	}
	a.log.WarnContext(ctx, "recurring automation: using expanded template fallback",
		"job_id", job.ID, "template_id", meta.TemplateID, "post_kind", meta.PostKind)
	return a.createRecurringAutomationPost(ctx, job, meta, content, aiCallbackResult{}, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) createRecurringAutomationPost(
	ctx context.Context,
	job domain.AIJob,
	meta *recurringAutomationMeta,
	content string,
	res aiCallbackResult,
	targetAccounts []string,
) error {
	if len(targetAccounts) == 0 {
		return fmt.Errorf("automation target accounts are empty")
	}
	if _, err := a.store.GetPostTemplate(ctx, job.TeamID, meta.TemplateID); err != nil {
		return fmt.Errorf("load automation template: %w", err)
	}
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
	recPost := domain.CreatePostInput{
		Title:                  domain.ResolveAutomationPostTitle(meta.PostTitle, res.Title),
		Content:                content,
		TargetAccounts:         targetAccounts,
		AccountContentOverride: a.aiAccountContentOverrides(ctx, job, res.AccountContentOverride, targetAccounts),
		ScheduledAt:            scheduledAt,
		Draft:                  draft,
		AuthorUserID:           &job.AuthorUserID,
		Source:                 domain.PostSourceAutomation,
		PostTemplateID:         &tplID,
		TemplateCounter:        &counterVal,
		TemplateOccurrenceAt:   meta.TemplateOccurrenceAt,
		TemplatePostRole:       role,
		UseVersions:            len(res.AccountContentOverride) > 0,
	}
	recPost.EnsureTitle()
	prepared, err := a.posts.Prepare(ctx, job.TeamID, recPost, postservice.Options{RequireTeam: true, CheckLimits: !draft})
	if err != nil {
		return fmt.Errorf("%w: %w", errAutomationPostValidation, err)
	}
	if err := postservice.ValidationError(prepared.Validation); err != nil {
		return fmt.Errorf("%w: %w", errAutomationPostValidation, err)
	}
	post, err := a.createJobAutomationPost(ctx, job, prepared.Input, map[string]string{"template_id": meta.TemplateID, "post_kind": meta.PostKind})
	if err != nil {
		return fmt.Errorf("create automation post: %w", err)
	}
	a.notifyReviewCreated(ctx, job.TeamID, post)
	return nil
}
