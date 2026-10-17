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
	"git.f4mily.net/goloom/internal/htmltext"
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
	articleText := htmltext.ExtractReadableText(strings.TrimSpace(item.Content))
	params := map[string]any{
		"occasion":            strings.TrimSpace(feed.PromptHint),
		"post_skeleton":       templateContent,
		"rss_article_title":   strings.TrimSpace(item.Title),
		"rss_article_content": articleText,
		"rss_article_link":    strings.TrimSpace(item.Link),
		"title_hint":          strings.TrimSpace(feed.TitleHint),
		"target_account_ids":  feed.TargetAccountIDs,
		"schedule":            false,
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

	var job domain.AIJob
	err = s.store.WithAuditTransaction(ctx, func(ctx context.Context) error {
		job, err = s.jobManager.SubmitJob(ctx, domain.AIJob{
			Origin:       domain.AIJobOriginAutomation,
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
		return nil
	})
	if err != nil {
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
	return s.store.WithAuditTransaction(ctx, func(ctx context.Context) error {
		if err := s.store.LockAutomationSource(ctx, "rss_feed", feed.ID); err != nil {
			return err
		}
		already, err := s.store.RSSItemAlreadyImported(ctx, feed.ID, itemKey)
		if err != nil || already {
			return err
		}
		return s.createRSSPostDirectOnce(ctx, feed, itemKey, content, title, scheduledAt, draft, ownerID)
	})
}

func (s *Service) createRSSPostDirectOnce(ctx context.Context, feed domain.RSSFeedConfig, itemKey, content, title string, scheduledAt time.Time, draft bool, ownerID string) error {
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
	input.EnsureTitle()
	principal := domain.AuthenticatedPrincipal{User: domain.User{ID: ownerID}, Kind: "system"}
	post, err := s.createAutomationPost(ctx, feed.TeamID, principal, input, map[string]string{"feed_id": feed.ID, "item_key_hash": sourceKey(itemKey)}, func(ctx context.Context, post domain.ScheduledPost) error {
		if err := s.store.RecordRSSImportedItem(ctx, feed.ID, itemKey, post.ID); err != nil {
			return err
		}
		return s.store.IncrementRSSFeedCounter(ctx, feed.TeamID, feed.ID)
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "rss import: post created", "feed_id", feed.ID, "post_id", post.ID, "status", post.Status)
	s.notifyReviewCreated(ctx, feed.TeamID, post)
	return nil
}
