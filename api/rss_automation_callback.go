package api

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) finishRSSAutomationFromAI(ctx context.Context, job domain.AIJob, rawResult json.RawMessage, meta *rssAutomationMeta) {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		_ = json.Unmarshal(rawResult, &res)
	}
	content := strings.TrimSpace(res.Content)
	if content == "" {
		a.finishRSSAutomationFallback(ctx, job, meta)
		return
	}
	a.createRSSAutomationPost(ctx, job, meta, content, res, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) finishRSSAutomationFallback(ctx context.Context, job domain.AIJob, meta *rssAutomationMeta) {
	content := meta.FallbackContent
	if content == "" {
		return
	}
	a.createRSSAutomationPost(ctx, job, meta, content, aiCallbackResult{}, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) createRSSAutomationPost(
	ctx context.Context,
	job domain.AIJob,
	meta *rssAutomationMeta,
	content string,
	res aiCallbackResult,
	targetAccounts []string,
) {
	if len(targetAccounts) == 0 {
		return
	}
	normalizedOverrides := domain.NormalizeAccountContentOverride(res.AccountContentOverride, targetAccounts)
	scheduledAt := rssAutomationScheduledAt(meta, res)
	draft := rssAutomationDraftFromMeta(meta)
	if meta.OutputMode == domain.AutomationOutputPublishNow {
		draft = false
		scheduledAt = time.Now().UTC()
	}

	feedID := meta.FeedID
	principal := domain.AuthenticatedPrincipal{
		User: domain.User{ID: job.AuthorUserID},
		Kind: "api_token",
	}
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
	post, err := a.store.CreateScheduledPost(ctx, job.TeamID, principal, rssPost)
	if err != nil {
		return
	}
	_ = a.store.UpdateRSSImportedItemPostID(ctx, meta.FeedID, meta.ItemKey, post.ID)
	_ = a.store.IncrementRSSFeedCounter(ctx, meta.FeedID)
}
