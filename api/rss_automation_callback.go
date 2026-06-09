package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) finishRSSAutomationFromAI(r *http.Request, job domain.AIJob, rawResult json.RawMessage, meta *rssAutomationMeta) {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		_ = json.Unmarshal(rawResult, &res)
	}
	content := strings.TrimSpace(res.Content)
	if content == "" {
		a.finishRSSAutomationFallback(r, job, meta)
		return
	}
	a.createRSSAutomationPost(r, job, meta, content, res, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) finishRSSAutomationFallback(r *http.Request, job domain.AIJob, meta *rssAutomationMeta) {
	content := meta.FallbackContent
	if content == "" {
		return
	}
	a.createRSSAutomationPost(r, job, meta, content, aiCallbackResult{}, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) createRSSAutomationPost(
	r *http.Request,
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
	post, err := a.store.CreateScheduledPost(r.Context(), job.TeamID, principal, domain.CreatePostInput{
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
	})
	if err != nil {
		return
	}
	_ = a.store.UpdateRSSImportedItemPostID(r.Context(), meta.FeedID, meta.ItemKey, post.ID)
	_ = a.store.IncrementRSSFeedCounter(r.Context(), meta.FeedID)
}
