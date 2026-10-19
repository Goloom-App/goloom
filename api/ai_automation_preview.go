package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/ai"
	"git.f4mily.net/goloom/internal/aicontext"
	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/htmltext"
	"git.f4mily.net/goloom/internal/netguard"
	"git.f4mily.net/goloom/internal/pagefetch"
	"git.f4mily.net/goloom/internal/rss"
	"git.f4mily.net/goloom/internal/scheduling"
	"github.com/jackc/pgx/v5"
)

const rssPreviewTimeout = 8 * time.Second
const rssPreviewItemLimit = 50

type rssPreviewItem struct {
	ItemKey     string    `json:"item_key"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Link        string    `json:"link"`
	PublishedAt time.Time `json:"published_at"`
}

func (a *API) fetchRSSPreviewItems(r *http.Request, feed domain.RSSFeedConfig) ([]rss.Item, error) {
	if a.rssPreviewFetch != nil {
		return a.rssPreviewFetch(r.Context(), feed.FeedURL)
	}
	return rss.NewParser().FetchWithPolicy(r.Context(), feed.FeedURL, netguard.NewPolicy(nil), rssPreviewTimeout)
}

// SetRSSPreviewFetcher installs a feed parser seam. Production uses guarded HTTP by default.
func (a *API) SetRSSPreviewFetcher(fetch func(context.Context, string) ([]rss.Item, error)) {
	a.rssPreviewFetch = fetch
}

func (a *API) handleListRSSPreviewItems(w http.ResponseWriter, r *http.Request) {
	teamID, feedID := r.PathValue("teamID"), r.PathValue("feedID")
	feed, err := a.store.GetRSSFeedConfigByID(r.Context(), teamID, feedID)
	if err != nil {
		a.writePreviewSourceStoreError(w, r, err, "rss_feed_not_found")
		return
	}
	items, err := a.fetchRSSPreviewItems(r, feed)
	if err != nil {
		a.writeError(w, r, "rss_preview_feed_unavailable", http.StatusBadGateway)
		return
	}
	out := make([]rssPreviewItem, 0, min(len(items), rssPreviewItemLimit))
	for _, item := range items {
		if len(out) == rssPreviewItemLimit {
			break
		}
		key := rss.ItemKey(item)
		if strings.TrimSpace(key) == "" {
			continue
		}
		out = append(out, rssPreviewItem{ItemKey: key, Title: item.Title, Content: htmltext.ExtractReadableText(item.Content), Link: item.Link, PublishedAt: item.PublishedAt.UTC()})
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

type rssAIPreviewRequest struct {
	ItemKey string            `json:"item_key"`
	Config  *rssPreviewConfig `json:"config,omitempty"`
}

type rssPreviewConfig struct {
	KnowledgeSourceIDs       domain.PatchField[[]string] `json:"knowledge_source_ids"`
	ContentTemplate          *string                     `json:"content_template,omitempty"`
	TitleTemplate            *string                     `json:"title_template,omitempty"`
	PromptHint               *string                     `json:"prompt_hint,omitempty"`
	EditorialGoal            *string                     `json:"editorial_goal,omitempty"`
	DesiredLength            *string                     `json:"desired_length,omitempty"`
	TitleHint                *string                     `json:"title_hint,omitempty"`
	TargetAccountIDs         *[]string                   `json:"target_account_ids,omitempty"`
	ArticleEnrichmentEnabled *bool                       `json:"article_enrichment_enabled,omitempty"`
}

func (a *API) handleRSSAIPreview(w http.ResponseWriter, r *http.Request) {
	var input rssAIPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.ItemKey) == "" {
		a.writeError(w, r, "invalid_rss_preview_item", http.StatusBadRequest)
		return
	}
	if input.Config != nil && ((input.Config.EditorialGoal != nil && !validRSSEditorialOptions(*input.Config.EditorialGoal, "")) ||
		(input.Config.DesiredLength != nil && !validRSSEditorialOptions("", *input.Config.DesiredLength))) {
		a.writeError(w, r, "invalid_rss_editorial_options", http.StatusBadRequest)
		return
	}
	teamID, feedID := r.PathValue("teamID"), r.PathValue("feedID")
	feed, err := a.store.GetRSSFeedConfigByID(r.Context(), teamID, feedID)
	if err != nil {
		a.writePreviewSourceStoreError(w, r, err, "rss_feed_not_found")
		return
	}
	feed = applyRSSPreviewConfig(feed, input.Config)
	if !a.validateKnowledgeSelection(w, r, feed.KnowledgeSourceIDs) {
		return
	}
	if len(feed.TargetAccountIDs) == 0 {
		a.writeError(w, r, "invalid_target_account_ids", http.StatusBadRequest)
		return
	}
	if _, _, err := a.posts.ResolveTargets(r.Context(), teamID, feed.TargetAccountIDs, nil); err != nil {
		a.writeError(w, r, "invalid_target_account_ids", http.StatusBadRequest)
		return
	}
	items, err := a.fetchRSSPreviewItems(r, feed)
	if err != nil {
		a.writeError(w, r, "rss_preview_feed_unavailable", http.StatusBadGateway)
		return
	}
	var selected *rss.Item
	for i := range items {
		if rss.ItemKey(items[i]) == input.ItemKey {
			selected = &items[i]
			break
		}
	}
	if selected == nil {
		a.writeError(w, r, "rss_preview_item_not_found", http.StatusNotFound)
		return
	}
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		a.writeError(w, r, "unauthorized", http.StatusUnauthorized)
		return
	}
	params := rssPreviewParams(feed, *selected)
	articleText, articleFetchFailed := rssPreviewArticleSource(r.Context(), feed, *selected)
	params["rss_article_content"] = articleText
	if articleFetchFailed {
		a.log.WarnContext(r.Context(), "rss AI preview: article enrichment fetch failed", "team_id", teamID, "feed_id", feed.ID, "item_key_hash", previewSourceKey(input.ItemKey))
	}
	job, err := a.submitAutomationPreview(r, principal.User.ID, teamID, params, map[string]any{
		"kind": "rss", "feed_id": feed.ID, "feed_name": feed.Name,
		"title": selected.Title, "content": htmltext.ExtractReadableText(selected.Content), "link": selected.Link, "article_fetch_failed": articleFetchFailed,
		"article_fetch_warning": previewArticleFetchWarning(articleFetchFailed),
	}, map[string]any{
		"content": params["post_skeleton"], "title": rss.ExpandTitle(feed.NormalizedTitleTemplate(), rss.ItemFields{Title: selected.Title, Link: selected.Link, Summary: selected.Content, FeedName: feed.Name, PublishedAt: selected.PublishedAt, Counter: feed.CounterNext}),
	}, params)
	if err != nil {
		a.writeAIJobSubmitError(w, r, err)
		return
	}
	auth.WriteJSON(w, http.StatusAccepted, aiTriggerResponse{JobID: job.ID, Status: job.Status})
}

func previewSourceKey(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func previewArticleFetchWarning(failed bool) string {
	if failed {
		return "article_fetch_failed; preview used feed content"
	}
	return ""
}

type recurringAIPreviewRequest struct {
	OccurrenceAt string                  `json:"occurrence_at"`
	PostKind     string                  `json:"post_kind,omitempty"`
	Config       *recurringPreviewConfig `json:"config,omitempty"`
}

type recurringPreviewConfig struct {
	Title                        *string   `json:"title,omitempty"`
	Content                      *string   `json:"content,omitempty"`
	RecurrenceJSON               *string   `json:"recurrence_json,omitempty"`
	CounterNext                  *int      `json:"counter_next,omitempty"`
	PromptHint                   *string   `json:"prompt_hint,omitempty"`
	TitleHint                    *string   `json:"title_hint,omitempty"`
	TargetAccountIDs             *[]string `json:"target_account_ids,omitempty"`
	AnnouncementEnabled          *bool     `json:"announcement_enabled,omitempty"`
	AnnouncementTitle            *string   `json:"announcement_title,omitempty"`
	AnnouncementContent          *string   `json:"announcement_content,omitempty"`
	AnnouncementDaysBefore       *int      `json:"announcement_days_before,omitempty"`
	AnnouncementCounterNext      *int      `json:"announcement_counter_next,omitempty"`
	AnnouncementTargetAccountIDs *[]string `json:"announcement_target_account_ids,omitempty"`
}

func (a *API) handleRecurringAIPreview(w http.ResponseWriter, r *http.Request) {
	var input recurringAIPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	occ, err := time.Parse(time.RFC3339, input.OccurrenceAt)
	if err != nil {
		a.writeError(w, r, "invalid_template_occurrence", http.StatusBadRequest)
		return
	}
	teamID, templateID := r.PathValue("teamID"), r.PathValue("templateID")
	tmpl, err := a.store.GetPostTemplate(r.Context(), teamID, templateID)
	if err != nil {
		a.writePreviewSourceStoreError(w, r, err, "post_template_not_found")
		return
	}
	if input.Config != nil && ((input.Config.CounterNext != nil && *input.Config.CounterNext < 1) || (input.Config.AnnouncementCounterNext != nil && *input.Config.AnnouncementCounterNext < 1)) {
		a.writeError(w, r, "invalid_template_counter", http.StatusBadRequest)
		return
	}
	tmpl = applyRecurringPreviewConfig(tmpl, input.Config)
	rule, err := scheduling.ParseRecurrenceJSON(tmpl.RecurrenceJSON)
	if err != nil {
		a.writeError(w, r, "invalid_template_recurrence", http.StatusBadRequest)
		return
	}
	concrete, err := scheduling.NextOccurrence(rule, occ.UTC().Add(-time.Second))
	if err != nil || !concrete.Equal(occ.UTC()) {
		a.writeError(w, r, "invalid_template_occurrence", http.StatusBadRequest)
		return
	}
	kind := input.PostKind
	if kind == "" {
		kind = "main"
	}
	if kind != "main" && kind != "announcement" {
		a.writeError(w, r, "invalid_template_post_kind", http.StatusBadRequest)
		return
	}
	var skipped bool
	if kind == "announcement" {
		skipped, err = a.store.IsPostTemplateAnnouncementSkipped(r.Context(), templateID, occ.UTC())
	} else {
		skipped, err = a.store.IsPostTemplateOccurrenceSkipped(r.Context(), templateID, occ.UTC())
	}
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	if skipped {
		a.writeError(w, r, "template_occurrence_skipped", http.StatusConflict)
		return
	}
	if kind == "announcement" && (!tmpl.AnnouncementEnabled || strings.TrimSpace(tmpl.AnnouncementContent) == "") {
		a.writeError(w, r, "template_announcement_unavailable", http.StatusBadRequest)
		return
	}
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		a.writeError(w, r, "unauthorized", http.StatusUnauthorized)
		return
	}
	content, title, targets, extra := recurringPreviewContent(tmpl, occ.UTC(), kind)
	if _, _, err := a.posts.ResolveTargets(r.Context(), teamID, targets, nil); err != nil {
		a.writeError(w, r, "invalid_ai_prompt_params", http.StatusBadRequest)
		return
	}
	params := map[string]any{"source_content": content, "prompt_hint": strings.TrimSpace(tmpl.PromptHint), "title_hint": strings.TrimSpace(tmpl.TitleHint), "target_account_ids": targets, "schedule": false, "recurring_post_kind": kind}
	for key, value := range extra {
		params[key] = value
	}
	job, err := a.submitAutomationPreview(r, principal.User.ID, teamID, params, map[string]any{
		"kind": "recurring", "template_id": tmpl.ID, "template_title": tmpl.Title, "occurrence_at": occ.UTC().Format(time.RFC3339), "post_kind": kind,
	}, map[string]any{
		"content": content, "title": title, "recurrence_json": tmpl.RecurrenceJSON,
		"counter_next": tmpl.CounterNext, "announcement_counter_next": tmpl.AnnouncementCounterNext,
	}, params)
	if err != nil {
		a.writeAIJobSubmitError(w, r, err)
		return
	}
	auth.WriteJSON(w, http.StatusAccepted, aiTriggerResponse{JobID: job.ID, Status: job.Status})
}

func rssPreviewParams(feed domain.RSSFeedConfig, item rss.Item) map[string]any {
	fields := rss.ItemFields{Title: item.Title, Link: item.Link, Summary: item.Content, FeedName: feed.Name, PublishedAt: item.PublishedAt, Counter: feed.CounterNext}
	params := map[string]any{
		"occasion":             strings.TrimSpace(feed.PromptHint),
		"post_skeleton":        rss.ExpandContent(feed.NormalizedContentTemplate(), fields),
		"rss_article_title":    item.Title,
		"rss_article_content":  htmltext.ExtractReadableText(item.Content),
		"rss_article_link":     item.Link,
		"title_hint":           strings.TrimSpace(feed.TitleHint),
		"target_account_ids":   feed.TargetAccountIDs,
		"schedule":             false,
		"knowledge_source_ids": feed.KnowledgeSourceIDs,
		"rss_editorial_goal":   domain.NormalizeRSSEditorialGoal(feed.EditorialGoal),
		"rss_desired_length":   domain.NormalizeRSSDesiredLength(feed.DesiredLength),
	}
	if domain.NormalizeRSSEditorialGoal(feed.EditorialGoal) == "teaser" {
		params["output_format"] = "teaser"
	}
	return params
}

func applyRSSPreviewConfig(feed domain.RSSFeedConfig, config *rssPreviewConfig) domain.RSSFeedConfig {
	if config == nil {
		return feed
	}
	if config.ContentTemplate != nil {
		feed.ContentTemplate = strings.TrimSpace(*config.ContentTemplate)
	}
	if config.TitleTemplate != nil {
		feed.TitleTemplate = strings.TrimSpace(*config.TitleTemplate)
	}
	if config.PromptHint != nil {
		feed.PromptHint = strings.TrimSpace(*config.PromptHint)
	}
	if config.KnowledgeSourceIDs.Set {
		feed.KnowledgeSourceIDs = config.KnowledgeSourceIDs.Value
	}
	if config.EditorialGoal != nil {
		feed.EditorialGoal = domain.NormalizeRSSEditorialGoal(*config.EditorialGoal)
	}
	if config.DesiredLength != nil {
		feed.DesiredLength = domain.NormalizeRSSDesiredLength(*config.DesiredLength)
	}
	if config.TitleHint != nil {
		feed.TitleHint = strings.TrimSpace(*config.TitleHint)
	}
	if config.TargetAccountIDs != nil {
		feed.TargetAccountIDs = domain.NormalizeMediaIDs(*config.TargetAccountIDs)
	}
	if config.ArticleEnrichmentEnabled != nil {
		feed.ArticleEnrichmentEnabled = *config.ArticleEnrichmentEnabled
	}
	return feed
}

func applyRecurringPreviewConfig(tmpl domain.PostTemplate, config *recurringPreviewConfig) domain.PostTemplate {
	if config == nil {
		return tmpl
	}
	if config.Title != nil {
		tmpl.Title = strings.TrimSpace(*config.Title)
	}
	if config.Content != nil {
		tmpl.Content = strings.TrimSpace(*config.Content)
	}
	if config.RecurrenceJSON != nil {
		tmpl.RecurrenceJSON = strings.TrimSpace(*config.RecurrenceJSON)
	}
	if config.CounterNext != nil {
		tmpl.CounterNext = *config.CounterNext
	}
	if config.PromptHint != nil {
		tmpl.PromptHint = strings.TrimSpace(*config.PromptHint)
	}
	if config.TitleHint != nil {
		tmpl.TitleHint = strings.TrimSpace(*config.TitleHint)
	}
	if config.TargetAccountIDs != nil {
		tmpl.TargetAccountIDs = domain.NormalizeMediaIDs(*config.TargetAccountIDs)
	}
	if config.AnnouncementEnabled != nil {
		tmpl.AnnouncementEnabled = *config.AnnouncementEnabled
	}
	if config.AnnouncementTitle != nil {
		tmpl.AnnouncementTitle = strings.TrimSpace(*config.AnnouncementTitle)
	}
	if config.AnnouncementContent != nil {
		tmpl.AnnouncementContent = strings.TrimSpace(*config.AnnouncementContent)
	}
	if config.AnnouncementDaysBefore != nil {
		tmpl.AnnouncementDaysBefore = *config.AnnouncementDaysBefore
	}
	if config.AnnouncementCounterNext != nil {
		tmpl.AnnouncementCounterNext = *config.AnnouncementCounterNext
	}
	if config.AnnouncementTargetAccountIDs != nil {
		tmpl.AnnouncementTargetAccountIDs = domain.NormalizeMediaIDs(*config.AnnouncementTargetAccountIDs)
	}
	return tmpl
}

func recurringPreviewContent(tmpl domain.PostTemplate, occ time.Time, kind string) (string, string, []string, map[string]any) {
	counter := tmpl.CounterNext
	content, title, targets := tmpl.Content, tmpl.Title, tmpl.TargetAccountIDs
	scheduledAt := occ.UTC()
	if tmpl.OutputMode == domain.AutomationOutputPublishNow {
		scheduledAt = time.Now().UTC()
	}
	extra := map[string]any{"main_event_at": occ.UTC().Format(time.RFC3339), "post_scheduled_at": scheduledAt.Format(time.RFC3339)}
	if kind == "announcement" {
		counter = tmpl.AnnouncementCounterNext
		days := tmpl.AnnouncementDaysBefore
		if days <= 0 {
			days = 2
		}
		announceAt := occ.Add(-time.Duration(days) * 24 * time.Hour)
		mainCounter := tmpl.CounterNext
		content = domain.ExpandDynamicVariables(tmpl.AnnouncementContent, announceAt, &counter, &occ, &mainCounter)
		title = domain.ExpandPostTemplateTitle(tmpl.AnnouncementTitle, announceAt, counter, &occ, &mainCounter)
		targets = tmpl.AnnouncementTargetAccountIDs
		if len(targets) == 0 {
			targets = tmpl.TargetAccountIDs
		}
		extra["days_before_main_event"] = days
		extra["post_scheduled_at"] = announceAt.UTC().Format(time.RFC3339)
	} else {
		content = tmpl.Content
		title = domain.ExpandPostTemplateTitle(title, occ, counter, nil, nil)
		if tmpl.AnnouncementEnabled && strings.TrimSpace(tmpl.AnnouncementContent) != "" {
			refContent, refTitle := expandedAnnouncementPreview(tmpl, occ)
			extra["announcement_reference_content"] = refContent
			extra["announcement_reference_title"] = refTitle
		}
	}
	return content, title, targets, extra
}

func expandedAnnouncementPreview(tmpl domain.PostTemplate, mainEventAt time.Time) (string, string) {
	daysBefore := tmpl.AnnouncementDaysBefore
	if daysBefore <= 0 {
		daysBefore = 2
	}
	announceAt := mainEventAt.Add(-time.Duration(daysBefore) * 24 * time.Hour)
	counter := tmpl.AnnouncementCounterNext
	if counter < 1 {
		counter = 1
	}
	mainCounter := tmpl.CounterNext
	if mainCounter < 1 {
		mainCounter = 1
	}
	content := domain.ExpandDynamicVariables(tmpl.AnnouncementContent, announceAt, &counter, &mainEventAt, &mainCounter)
	title := domain.ExpandPostTemplateTitle(tmpl.AnnouncementTitle, announceAt, counter, &mainEventAt, &mainCounter)
	return content, title
}

func rssPreviewArticleSource(ctx context.Context, feed domain.RSSFeedConfig, item rss.Item) (string, bool) {
	feedText := htmltext.ExtractReadableText(strings.TrimSpace(item.Content))
	if !feed.ArticleEnrichmentEnabled || strings.TrimSpace(item.Link) == "" || len([]rune(feedText)) >= 280 {
		return feedText, false
	}
	text, err := pagefetch.FetchText(ctx, item.Link, netguard.NewPolicy(nil), rssPreviewTimeout)
	if err != nil {
		return feedText, true
	}
	return text, false
}

func (a *API) submitAutomationPreview(r *http.Request, authorID, teamID string, params map[string]any, source, template, inputs map[string]any) (domain.AIJob, error) {
	params["no_publish"] = true
	rawParams, err := json.Marshal(params)
	if err != nil {
		return domain.AIJob{}, err
	}
	aiCtx, err := aicontext.Build(r.Context(), a.store, teamID, aicontext.Full())
	if err != nil {
		return domain.AIJob{}, err
	}
	aiCtx, err = aicontext.Select(aiCtx, rawParams)
	if err != nil {
		return domain.AIJob{}, err
	}
	if _, _, err := ai.PrepareVoicePrompt(aiCtx, rawParams); err != nil {
		return domain.AIJob{}, errors.New("invalid_ai_prompt_params")
	}
	previewRaw, err := json.Marshal(map[string]any{"source": source, "template": template, "generation_inputs": inputs})
	if err != nil {
		return domain.AIJob{}, err
	}
	payload, err := json.Marshal(map[string]json.RawMessage{"params": rawParams, "preview": previewRaw})
	if err != nil {
		return domain.AIJob{}, err
	}
	return a.jobManager.SubmitJob(r.Context(), domain.AIJob{Origin: domain.AIJobOriginPreview, TeamID: teamID, AuthorUserID: authorID, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
}

func (a *API) writeAIJobSubmitError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, aicontext.ErrKnowledgeSourceUnavailable):
		a.writeError(w, r, "knowledge_source_unavailable", http.StatusBadRequest)
	case errors.Is(err, aijobs.ErrAIServiceNotConfigured):
		a.writeError(w, r, "ai_service_not_configured", http.StatusUnprocessableEntity)
	case err.Error() == "invalid_ai_prompt_params":
		a.writeError(w, r, "invalid_ai_prompt_params", http.StatusBadRequest)
	default:
		a.log.ErrorContext(r.Context(), "submit AI preview job failed", "team_id", r.PathValue("teamID"), "error", err)
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
	}
}

func (a *API) writePreviewSourceStoreError(w http.ResponseWriter, r *http.Request, err error, notFoundCode string) {
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
		a.writeError(w, r, notFoundCode, http.StatusNotFound)
		return
	}
	a.writeStoreError(w, r, err, "")
}
