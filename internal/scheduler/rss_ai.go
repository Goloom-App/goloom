package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/rss"
)

func (s *Service) shouldEnhanceRSSWithAI(ctx context.Context, feed domain.RSSFeedConfig) bool {
	if !feed.AiEnhanceEnabled || s.jobManager == nil {
		return false
	}
	team, err := s.store.GetTeamByID(ctx, feed.TeamID)
	if err != nil || !team.IsAIEnabled {
		return false
	}
	return true
}

func (s *Service) submitRSSAIEnhancement(
	ctx context.Context,
	feed domain.RSSFeedConfig,
	item rss.Item,
	itemKey string,
	templateContent string,
	title string,
	scheduledAt time.Time,
	draft bool,
	ownerID string,
) error {
	params := map[string]any{
		"refine_content":       true,
		"source_content":       templateContent,
		"rss_article_title":    strings.TrimSpace(item.Title),
		"rss_article_content":  strings.TrimSpace(item.Content),
		"rss_article_link":     strings.TrimSpace(item.Link),
		"prompt_hint":        strings.TrimSpace(feed.PromptHint),
		"title_hint":         strings.TrimSpace(feed.TitleHint),
		"target_account_ids": feed.TargetAccountIDs,
		"schedule":             false,
		"rss_automation": map[string]any{
			"feed_id":          feed.ID,
			"item_key":         itemKey,
			"output_mode":      string(feed.OutputMode),
			"scheduled_at":     scheduledAt.UTC().Format(time.RFC3339),
			"draft":            draft,
			"post_title":       title,
			"fallback_content": templateContent,
		},
	}
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		Params json.RawMessage `json:"params"`
	}{Params: paramsRaw})
	if err != nil {
		return err
	}

	job, err := s.jobManager.SubmitJob(ctx, domain.AIJob{
		TeamID:       feed.TeamID,
		AuthorUserID: ownerID,
		Type:         domain.AIJobTypeVoiceEngine,
		Payload:      payload,
	})
	if err != nil {
		if errors.Is(err, aijobs.ErrAIServiceNotConfigured) {
			return err
		}
		return fmt.Errorf("submit rss ai job: %w", err)
	}
	if job.Status == domain.AIJobStatusFailed {
		return fmt.Errorf("rss ai job dispatch failed")
	}
	if err := s.store.RecordRSSImportedItem(ctx, feed.ID, itemKey, ""); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "rss import: ai enhancement queued", "feed_id", feed.ID, "job_id", job.ID, "item_key", itemKey)
	return nil
}

func (s *Service) createRSSPostDirect(
	ctx context.Context,
	feed domain.RSSFeedConfig,
	itemKey string,
	content string,
	title string,
	scheduledAt time.Time,
	draft bool,
	ownerID string,
) error {
	feedID := feed.ID
	input := domain.CreatePostInput{
		Title:          title,
		Content:        content,
		ScheduledAt:    scheduledAt,
		TargetAccounts: feed.TargetAccountIDs,
		Draft:          draft,
		AuthorUserID:   &ownerID,
		Source:         domain.PostSourceAutomation,
		RSSFeedID:      &feedID,
	}
	principal := domain.AuthenticatedPrincipal{User: domain.User{ID: ownerID}, Kind: "system"}
	post, err := s.store.CreateScheduledPost(ctx, feed.TeamID, principal, input)
	if err != nil {
		return err
	}
	if err := s.store.RecordRSSImportedItem(ctx, feed.ID, itemKey, post.ID); err != nil {
		return err
	}
	if err := s.store.IncrementRSSFeedCounter(ctx, feed.ID); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "rss import: post created", "feed_id", feed.ID, "post_id", post.ID, "status", post.Status)
	return nil
}
