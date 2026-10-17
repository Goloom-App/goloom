package api

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) finishRSSAutomationFromAI(ctx context.Context, job domain.AIJob, rawResult json.RawMessage, meta *rssAutomationMeta) error {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		_ = json.Unmarshal(rawResult, &res)
	}
	content := strings.TrimSpace(res.Content)
	if content == "" {
		return a.finishRSSAutomationFallback(ctx, job, meta)
	}
	return a.createRSSAutomationPost(ctx, job, meta, content, res, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) finishRSSAutomationFallback(ctx context.Context, job domain.AIJob, meta *rssAutomationMeta) error {
	content := meta.FallbackContent
	if content == "" {
		return nil
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
		return nil
	}
	normalizedOverrides := domain.NormalizeAccountContentOverride(res.AccountContentOverride, targetAccounts)
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
		AccountContentOverride: normalizedOverrides,
		ScheduledAt:            scheduledAt,
		Draft:                  draft,
		AuthorUserID:           &job.AuthorUserID,
		Source:                 domain.PostSourceAutomation,
		RSSFeedID:              &feedID,
		UseVersions:            len(normalizedOverrides) > 0,
	}
	rssPost.EnsureTitle()
	post, err := a.createJobAutomationPost(ctx, job, rssPost, map[string]string{"feed_id": meta.FeedID})
	if err != nil {
		return err
	}
	a.notifyReviewCreated(ctx, job.TeamID, post)
	if err := a.store.UpdateRSSImportedItemPostID(ctx, meta.FeedID, meta.ItemKey, post.ID); err != nil {
		return err
	}
	return a.store.IncrementRSSFeedCounter(ctx, meta.FeedID)
}
