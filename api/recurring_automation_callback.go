package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) finishRecurringAutomationFromAI(r *http.Request, job domain.AIJob, rawResult json.RawMessage, meta *recurringAutomationMeta) {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		_ = json.Unmarshal(rawResult, &res)
	}
	content := strings.TrimSpace(res.Content)
	if content == "" {
		a.finishRecurringAutomationFallback(r, job, meta)
		return
	}
	a.createRecurringAutomationPost(r, job, meta, content, res, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) finishRecurringAutomationFallback(r *http.Request, job domain.AIJob, meta *recurringAutomationMeta) {
	content := meta.FallbackContent
	if content == "" {
		return
	}
	a.createRecurringAutomationPost(r, job, meta, content, aiCallbackResult{}, targetAccountIDsFromJobPayload(job.Payload))
}

func (a *API) createRecurringAutomationPost(
	r *http.Request,
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
	if meta.OutputMode == domain.AutomationOutputPublishNow {
		draft = false
		scheduledAt = time.Now().UTC()
	}

	tplID := meta.TemplateID
	counterVal := meta.TemplateCounter
	principal := domain.AuthenticatedPrincipal{
		User: domain.User{ID: job.AuthorUserID},
		Kind: "api_token",
	}
	_, err := a.store.CreateScheduledPost(r.Context(), job.TeamID, principal, domain.CreatePostInput{
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
		UseVersions:            len(normalizedOverrides) > 0,
	})
	if err != nil {
		return
	}
}
