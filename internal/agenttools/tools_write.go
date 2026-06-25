package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postservice"
)

// roleEditor is the role set every write tool requires.
var roleEditor = []domain.TeamRole{domain.RoleEditor, domain.RoleOwner}

func coreCreateCampaign(ctx context.Context, d Deps, inv Invocation, in CreateCampaignInput) (CreateCampaignOutput, error) {
	if err := requireScope(inv, auth.ScopeWrite); err != nil {
		return CreateCampaignOutput{}, err
	}
	if err := requireTeam(ctx, d, inv, in.TeamID, roleEditor...); err != nil {
		return CreateCampaignOutput{}, err
	}
	if strings.TrimSpace(in.TeamID) == "" {
		return CreateCampaignOutput{}, fmt.Errorf("team_id is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return CreateCampaignOutput{}, fmt.Errorf("name is required")
	}

	structure, _ := json.Marshal(in.Structure)
	created, err := d.Store.CreateCampaignFormat(ctx, in.TeamID, domain.CampaignFormat{
		TeamID:           in.TeamID,
		Name:             in.Name,
		Structure:        structure,
		RequiredHashtags: in.RequiredHashtags,
		IsActive:         true,
	})
	if err != nil {
		return CreateCampaignOutput{}, err
	}
	d.recordAudit(ctx, inv, in.TeamID, "campaign.create", "campaign", created.ID, "Created campaign: "+created.Name)
	return CreateCampaignOutput{CampaignID: created.ID, Name: created.Name}, nil
}

func coreCreateRecurring(ctx context.Context, d Deps, inv Invocation, in CreateRecurringInput) (CreateRecurringOutput, error) {
	if err := requireScope(inv, auth.ScopeWrite); err != nil {
		return CreateRecurringOutput{}, err
	}
	if err := requireTeam(ctx, d, inv, in.TeamID, roleEditor...); err != nil {
		return CreateRecurringOutput{}, err
	}
	if strings.TrimSpace(in.TeamID) == "" {
		return CreateRecurringOutput{}, fmt.Errorf("team_id is required")
	}
	if strings.TrimSpace(in.Content) == "" {
		return CreateRecurringOutput{}, fmt.Errorf("content is required")
	}
	if len(in.TargetAccounts) == 0 {
		return CreateRecurringOutput{}, fmt.Errorf("target_accounts is required")
	}
	if _, _, err := d.Posts.ResolveTargets(ctx, in.TeamID, in.TargetAccounts, nil); err != nil {
		return CreateRecurringOutput{}, err
	}

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	template, err := d.Store.CreatePostTemplate(ctx, in.TeamID, inv.Principal, domain.CreatePostTemplateInput{
		Title:            strings.TrimSpace(in.Title),
		Content:          strings.TrimSpace(in.Content),
		RecurrenceJSON:   in.RecurrenceJSON,
		TargetAccountIDs: in.TargetAccounts,
		Visibility:       domain.NormalizePostVisibility(in.Visibility),
		Enabled:          &enabled,
	})
	if err != nil {
		return CreateRecurringOutput{}, err
	}
	d.recordAudit(ctx, inv, in.TeamID, "recurring.create", "recurring", template.ID, "Created recurring template: "+strings.TrimSpace(in.Title))
	return CreateRecurringOutput{TemplateID: template.ID}, nil
}

func coreCreateRSSFeed(ctx context.Context, d Deps, inv Invocation, in CreateRSSFeedInput) (CreateRSSFeedOutput, error) {
	if err := requireScope(inv, auth.ScopeWrite); err != nil {
		return CreateRSSFeedOutput{}, err
	}
	if err := requireTeam(ctx, d, inv, in.TeamID, roleEditor...); err != nil {
		return CreateRSSFeedOutput{}, err
	}
	if strings.TrimSpace(in.TeamID) == "" {
		return CreateRSSFeedOutput{}, fmt.Errorf("team_id is required")
	}
	if err := validateFeedURL(in.FeedURL); err != nil {
		return CreateRSSFeedOutput{}, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return CreateRSSFeedOutput{}, fmt.Errorf("name is required")
	}
	if len(in.TargetAccountIDs) == 0 {
		return CreateRSSFeedOutput{}, fmt.Errorf("target_account_ids is required")
	}
	if _, _, err := d.Posts.ResolveTargets(ctx, in.TeamID, in.TargetAccountIDs, nil); err != nil {
		return CreateRSSFeedOutput{}, err
	}

	created, err := d.Store.CreateRSSFeedConfig(ctx, in.TeamID, domain.RSSFeedConfig{
		TeamID:           in.TeamID,
		FeedURL:          strings.TrimSpace(in.FeedURL),
		Name:             strings.TrimSpace(in.Name),
		ContentTemplate:  in.ContentTemplate,
		TargetAccountIDs: in.TargetAccountIDs,
		OutputMode:       domain.NormalizeAutomationOutputMode(in.OutputMode),
		IsActive:         true,
	})
	if err != nil {
		return CreateRSSFeedOutput{}, err
	}
	d.recordAudit(ctx, inv, in.TeamID, "rss_feed.create", "rss_feed", created.ID, "Created RSS feed: "+strings.TrimSpace(in.Name))
	return CreateRSSFeedOutput{FeedID: created.ID}, nil
}

func coreSchedulePost(ctx context.Context, d Deps, inv Invocation, in SchedulePostInput) (SchedulePostOutput, error) {
	if err := requireScope(inv, auth.ScopeWriteSchedule); err != nil {
		return SchedulePostOutput{}, err
	}
	if err := requireTeam(ctx, d, inv, in.TeamID, roleEditor...); err != nil {
		return SchedulePostOutput{}, err
	}
	scheduledAt, err := time.Parse(time.RFC3339, in.ScheduledAt)
	if err != nil {
		return SchedulePostOutput{}, fmt.Errorf("invalid scheduled_at: %w", err)
	}

	prepared, err := d.Posts.Prepare(ctx, in.TeamID, domain.CreatePostInput{
		Title:                  in.Title,
		Content:                in.Content,
		ScheduledAt:            scheduledAt,
		TargetAccounts:         in.TargetAccounts,
		Visibility:             in.Visibility,
		AccountContentOverride: in.AccountContentOverride,
	}, postservice.Options{CheckLimits: true, RequireTeam: true})
	if err != nil {
		return SchedulePostOutput{}, err
	}
	if err := postservice.ValidationError(prepared.Validation); err != nil {
		return SchedulePostOutput{}, err
	}

	post, err := d.Store.CreateScheduledPost(ctx, prepared.EffectiveTeam, inv.Principal, prepared.Input)
	if err != nil {
		return SchedulePostOutput{}, err
	}
	d.recordAudit(ctx, inv, prepared.EffectiveTeam, "post.create", "post", post.ID, "Scheduled post: "+post.Title)
	return SchedulePostOutput{
		PostID:      post.ID,
		ScheduledAt: post.ScheduledAt.Format(time.RFC3339),
		Status:      string(post.Status),
	}, nil
}

func coreDraftPost(ctx context.Context, d Deps, inv Invocation, in DraftPostInput) (DraftPostOutput, error) {
	if err := requireScope(inv, auth.ScopeWriteDraft); err != nil {
		return DraftPostOutput{}, err
	}
	if err := requireTeam(ctx, d, inv, in.TeamID, roleEditor...); err != nil {
		return DraftPostOutput{}, err
	}

	// Drafts still require a title and validated targets/overrides, but skip
	// character-limit enforcement (they are refined before scheduling).
	prepared, err := d.Posts.Prepare(ctx, in.TeamID, domain.CreatePostInput{
		Title:                  in.Title,
		Content:                in.Content,
		TargetAccounts:         in.TargetAccounts,
		Visibility:             in.Visibility,
		Draft:                  true,
		AccountContentOverride: in.AccountContentOverride,
	}, postservice.Options{CheckLimits: false, RequireTeam: true})
	if err != nil {
		return DraftPostOutput{}, err
	}

	post, err := d.Store.CreateScheduledPost(ctx, prepared.EffectiveTeam, inv.Principal, prepared.Input)
	if err != nil {
		return DraftPostOutput{}, err
	}
	d.recordAudit(ctx, inv, prepared.EffectiveTeam, "post.create", "post", post.ID, "Drafted post: "+post.Title)
	return DraftPostOutput{
		PostID:         post.ID,
		Status:         string(post.Status),
		Title:          post.Title,
		Content:        post.Content,
		TargetAccounts: post.TargetAccounts,
		ScheduledAt:    post.ScheduledAt.Format(time.RFC3339),
	}, nil
}

func coreModifyPost(ctx context.Context, d Deps, inv Invocation, in ModifyPostInput) (ModifyPostOutput, error) {
	if err := requireScope(inv, auth.ScopeWrite); err != nil {
		return ModifyPostOutput{}, err
	}

	existing, err := d.Store.GetScheduledPostByID(ctx, in.PostID)
	if err != nil {
		return ModifyPostOutput{}, err
	}
	if err := requireTeam(ctx, d, inv, existing.TeamID, roleEditor...); err != nil {
		return ModifyPostOutput{}, err
	}

	patch := domain.UpdatePostPatch{}
	if in.Title != nil {
		patch.Title = domain.PatchField[string]{Value: *in.Title, Set: true}
	}
	if in.Content != nil {
		patch.Content = domain.PatchField[string]{Value: *in.Content, Set: true}
	}
	if in.ScheduledAt != nil {
		t, err := time.Parse(time.RFC3339, *in.ScheduledAt)
		if err != nil {
			return ModifyPostOutput{}, fmt.Errorf("invalid scheduled_at: %w", err)
		}
		patch.ScheduledAt = domain.PatchField[time.Time]{Value: t, Set: true}
	}
	if in.Visibility != nil {
		patch.Visibility = domain.PatchField[string]{Value: domain.NormalizePostVisibility(*in.Visibility), Set: true}
	}
	if in.TargetAccounts != nil {
		patch.TargetAccounts = domain.PatchField[[]string]{Value: *in.TargetAccounts, Set: true}
	}
	if in.AccountContentOverride != nil {
		patch.AccountContentOverride = domain.PatchField[map[string]string]{Value: in.AccountContentOverride, Set: true}
	}

	// Validate the post as it will look after the patch. Loading existing
	// versions lets ApplyPostPatch reconstruct the effective per-account content
	// so character limits are checked against the real destinations.
	versions, err := d.Store.ListPostVersionsForTeamPost(ctx, existing.TeamID, in.PostID)
	if err != nil {
		return ModifyPostOutput{}, err
	}
	merged, _ := domain.ApplyPostPatch(existing, versions, patch)
	if in.AccountContentOverride != nil {
		merged.AccountContentOverride = in.AccountContentOverride
	}

	prepared, err := d.Posts.Prepare(ctx, existing.TeamID, merged, postservice.Options{CheckLimits: !merged.Draft, RequireTeam: true})
	if err != nil {
		return ModifyPostOutput{}, err
	}
	if err := postservice.ValidationError(prepared.Validation); err != nil {
		return ModifyPostOutput{}, err
	}

	updated, err := d.Store.PatchScheduledPost(ctx, existing.TeamID, in.PostID, patch)
	if err != nil {
		return ModifyPostOutput{}, err
	}
	d.recordAudit(ctx, inv, existing.TeamID, "post.update", "post", in.PostID, "Updated post: "+prepared.Input.Title)
	return ModifyPostOutput{
		PostID:         in.PostID,
		Status:         string(updated.Status),
		Title:          updated.Title,
		Content:        updated.Content,
		TargetAccounts: updated.TargetAccounts,
		ScheduledAt:    updated.ScheduledAt.Format(time.RFC3339),
	}, nil
}

func coreDeletePost(ctx context.Context, d Deps, inv Invocation, in DeletePostInput) (DeletePostOutput, error) {
	if err := requireScope(inv, auth.ScopeDelete); err != nil {
		return DeletePostOutput{}, err
	}
	existing, err := d.Store.GetScheduledPostByID(ctx, in.PostID)
	if err != nil {
		return DeletePostOutput{}, err
	}
	if err := requireTeam(ctx, d, inv, existing.TeamID, roleEditor...); err != nil {
		return DeletePostOutput{}, err
	}
	if err := d.Store.DeleteScheduledPost(ctx, existing.TeamID, in.PostID); err != nil {
		return DeletePostOutput{}, err
	}
	d.recordAudit(ctx, inv, existing.TeamID, "post.delete", "post", in.PostID, "Deleted post: "+existing.Title)
	return DeletePostOutput{Success: true}, nil
}
