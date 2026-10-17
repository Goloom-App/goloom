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

func (a *API) finishRSSAutomationFromAI(ctx context.Context, job domain.AIJob, rawResult json.RawMessage, meta *rssAutomationMeta) error {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		if err := json.Unmarshal(rawResult, &res); err != nil {
			if fallbackErr := a.finishRSSAutomationFallback(ctx, job, meta); fallbackErr != nil {
				return fmt.Errorf("decode automation result: %w; fallback failed: %w", err, fallbackErr)
			}
			return fmt.Errorf("decode automation result: %w; %w", err, errAutomationTemplateFallback)
		}
	}
	content := strings.TrimSpace(res.Content)
	if content == "" {
		if err := a.finishRSSAutomationFallback(ctx, job, meta); err != nil {
			return err
		}
		return fmt.Errorf("empty AI content; %w", errAutomationTemplateFallback)
	}
	err := a.createRSSAutomationPost(ctx, job, meta, content, res, targetAccountIDsFromJobPayload(job.Payload))
	if !errors.Is(err, errAutomationPostValidation) {
		return err
	}
	a.log.WarnContext(ctx, "rss automation: invalid AI content, trying template fallback", "job_id", job.ID, "error", err)
	if fallbackErr := a.finishRSSAutomationFallback(ctx, job, meta); fallbackErr != nil {
		return fmt.Errorf("AI post validation: %v; fallback failed: %w", err, fallbackErr)
	}
	return fmt.Errorf("AI post validation: %v; %w", err, errAutomationTemplateFallback)
}

func (a *API) finishRSSAutomationFallback(ctx context.Context, job domain.AIJob, meta *rssAutomationMeta) error {
	content := meta.FallbackContent
	if content == "" {
		return fmt.Errorf("%w: automation fallback content is empty", errAutomationPostNotCreated)
	}
	return a.createRSSAutomationPost(ctx, job, meta, content, aiCallbackResult{}, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) createRSSAutomationPost(
	ctx context.Context,
	job domain.AIJob,
	meta *rssAutomationMeta,
	content string,
	res aiCallbackResult,
	targetAccounts []string,
) error {
	if len(targetAccounts) == 0 {
		return fmt.Errorf("%w: automation target accounts are empty", errAutomationPostNotCreated)
	}
	if _, err := a.store.GetRSSFeedConfigByID(ctx, job.TeamID, meta.FeedID); err != nil {
		return fmt.Errorf("%w: load automation feed: %w", errAutomationPostNotCreated, err)
	}
	scheduledAt := rssAutomationScheduledAt(meta, res)
	draft := rssAutomationDraftFromMeta(meta)
	if meta.OutputMode == domain.AutomationOutputPublishNow {
		draft = false
		scheduledAt = time.Now().UTC()
	}

	feedID := meta.FeedID
	rssPost := domain.CreatePostInput{
		Title:                  domain.ResolveAutomationPostTitle(meta.PostTitle, res.Title),
		Content:                content,
		TargetAccounts:         targetAccounts,
		AccountContentOverride: a.aiAccountContentOverrides(ctx, job, res.AccountContentOverride, targetAccounts),
		ScheduledAt:            scheduledAt,
		Draft:                  draft,
		AuthorUserID:           &job.AuthorUserID,
		Source:                 domain.PostSourceAutomation,
		RSSFeedID:              &feedID,
		RSSItemKey:             meta.ItemKey,
		UseVersions:            len(res.AccountContentOverride) > 0,
	}
	rssPost.EnsureTitle()
	prepared, err := a.posts.Prepare(ctx, job.TeamID, rssPost, postservice.Options{RequireTeam: true, CheckLimits: !draft})
	if err != nil {
		return fmt.Errorf("%w: %w: %w", errAutomationPostNotCreated, errAutomationPostValidation, err)
	}
	if err := postservice.ValidationError(prepared.Validation); err != nil {
		return fmt.Errorf("%w: %w: %w", errAutomationPostNotCreated, errAutomationPostValidation, err)
	}
	post, err := a.createJobAutomationPost(ctx, job, prepared.Input, map[string]string{"feed_id": meta.FeedID})
	if err != nil {
		return fmt.Errorf("%w: create automation post: %w", errAutomationPostNotCreated, err)
	}
	a.notifyReviewCreated(ctx, job.TeamID, post)
	if err := a.store.UpdateRSSImportedItemPostID(ctx, job.TeamID, meta.FeedID, meta.ItemKey, post.ID); err != nil {
		return fmt.Errorf("link automation RSS item: %w", err)
	}
	if err := a.store.IncrementRSSFeedCounter(ctx, job.TeamID, meta.FeedID); err != nil {
		return fmt.Errorf("increment automation RSS counter: %w", err)
	}
	return nil
}
