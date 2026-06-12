package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"git.f4mily.net/goloom/internal/domain"
)

// ===== Campaigns =====

func (h *Handler) handleCreateCampaign(ctx context.Context, req *mcp.CallToolRequest, input CreateCampaignInput) (*mcp.CallToolResult, CreateCampaignOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, CreateCampaignOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleEditor)
	if err != nil || !allowed {
		return nil, CreateCampaignOutput{}, fmt.Errorf("forbidden")
	}

	structure, _ := json.Marshal(input.Structure)
	campaign := domain.CampaignFormat{
		TeamID:           input.TeamID,
		Name:             input.Name,
		Structure:        structure,
		RequiredHashtags: input.RequiredHashtags,
		IsActive:         true,
	}

	created, err := h.store.CreateCampaignFormat(ctx, input.TeamID, campaign)
	if err != nil {
		return nil, CreateCampaignOutput{}, err
	}

	return nil, CreateCampaignOutput{
		CampaignID: created.ID,
		Name:       created.Name,
	}, nil
}

func (h *Handler) handleGetCampaign(ctx context.Context, req *mcp.CallToolRequest, input GetCampaignInput) (*mcp.CallToolResult, GetCampaignOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, GetCampaignOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleViewer)
	if err != nil || !allowed {
		return nil, GetCampaignOutput{}, fmt.Errorf("forbidden")
	}

	campaign, err := h.store.GetCampaignFormatByID(ctx, input.TeamID, input.CampaignID)
	if err != nil {
		return nil, GetCampaignOutput{}, err
	}

	return nil, GetCampaignOutput{
		CampaignID:       campaign.ID,
		Name:             campaign.Name,
		Weekday:          campaign.Weekday,
		Structure:        string(campaign.Structure),
		RequiredHashtags: campaign.RequiredHashtags,
		IsActive:         campaign.IsActive,
	}, nil
}

// ===== Recurring Posts =====

func (h *Handler) handleCreateRecurring(ctx context.Context, req *mcp.CallToolRequest, input CreateRecurringInput) (*mcp.CallToolResult, CreateRecurringOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, CreateRecurringOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleEditor)
	if err != nil || !allowed {
		return nil, CreateRecurringOutput{}, fmt.Errorf("forbidden")
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	createInput := domain.CreatePostTemplateInput{
		Title:            input.Title,
		Content:          input.Content,
		RecurrenceJSON:   input.RecurrenceJSON,
		TargetAccountIDs: input.TargetAccounts,
		Visibility:       domain.NormalizePostVisibility(input.Visibility),
		Enabled:          &enabled,
	}

	template, err := h.store.CreatePostTemplate(ctx, input.TeamID, *principal, createInput)
	if err != nil {
		return nil, CreateRecurringOutput{}, err
	}

	return nil, CreateRecurringOutput{
		TemplateID: template.ID,
	}, nil
}

// ===== RSS Feeds =====

func (h *Handler) handleCreateRSSFeed(ctx context.Context, req *mcp.CallToolRequest, input CreateRSSFeedInput) (*mcp.CallToolResult, CreateRSSFeedOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, CreateRSSFeedOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleEditor)
	if err != nil || !allowed {
		return nil, CreateRSSFeedOutput{}, fmt.Errorf("forbidden")
	}

	feed := domain.RSSFeedConfig{
		TeamID:           input.TeamID,
		FeedURL:          input.FeedURL,
		Name:             input.Name,
		ContentTemplate:  input.ContentTemplate,
		TargetAccountIDs: input.TargetAccountIDs,
		OutputMode:       domain.NormalizeAutomationOutputMode(input.OutputMode),
		IsActive:         true,
	}

	created, err := h.store.CreateRSSFeedConfig(ctx, input.TeamID, feed)
	if err != nil {
		return nil, CreateRSSFeedOutput{}, err
	}

	return nil, CreateRSSFeedOutput{
		FeedID: created.ID,
	}, nil
}

// ===== Calendar =====

func (h *Handler) handleGetCalendar(ctx context.Context, req *mcp.CallToolRequest, input GetCalendarInput) (*mcp.CallToolResult, GetCalendarOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, GetCalendarOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleViewer)
	if err != nil || !allowed {
		return nil, GetCalendarOutput{}, fmt.Errorf("forbidden")
	}

	posts, err := h.store.ListTeamPosts(ctx, input.TeamID)
	if err != nil {
		return nil, GetCalendarOutput{}, err
	}

	// Parse date range
	from := time.Now().AddDate(0, 0, -7)
	to := time.Now().AddDate(0, 0, 7)
	if input.FromDate != "" {
		if t, err := time.Parse(time.RFC3339, input.FromDate); err == nil {
			from = t
		}
	}
	if input.ToDate != "" {
		if t, err := time.Parse(time.RFC3339, input.ToDate); err == nil {
			to = t
		}
	}

	var result []CalendarPost
	for _, p := range posts {
		if p.ScheduledAt.After(from) && p.ScheduledAt.Before(to) {
			result = append(result, CalendarPost{
				ID:          p.ID,
				Title:       p.Title,
				Content:     TruncateString(p.Content, 100),
				ScheduledAt: p.ScheduledAt.Format(time.RFC3339),
				Status:      string(p.Status),
			})
		}
	}

	return nil, GetCalendarOutput{Posts: result, Total: len(result)}, nil
}

// ===== Free Slot Finding =====

func (h *Handler) handleFindFreeSlot(ctx context.Context, req *mcp.CallToolRequest, input FindFreeSlotInput) (*mcp.CallToolResult, FindFreeSlotOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, FindFreeSlotOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleViewer)
	if err != nil || !allowed {
		return nil, FindFreeSlotOutput{}, fmt.Errorf("forbidden")
	}

	// Parse dates
	after := time.Now().UTC()
	if input.AfterDate != "" {
		if t, err := time.Parse(time.RFC3339, input.AfterDate); err == nil {
			after = t
		}
	}
	before := after.AddDate(0, 0, 30)
	if input.BeforeDate != "" {
		if t, err := time.Parse(time.RFC3339, input.BeforeDate); err == nil {
			before = t
		}
	}

	targetWeekday := ParseWeekday(input.Weekday)

	// Get existing posts
	posts, err := h.store.ListTeamPosts(ctx, input.TeamID)
	if err != nil {
		return nil, FindFreeSlotOutput{}, err
	}

	// Find next free slot
	date, found := FindNextFreeSlot(posts, after, before, targetWeekday)
	if !found {
		return nil, FindFreeSlotOutput{Available: false}, nil
	}

	return nil, FindFreeSlotOutput{
		Date:      date.Format(time.RFC3339),
		Weekday:   date.Weekday().String(),
		Available: true,
	}, nil
}

// ===== Schedule Post =====

func (h *Handler) handleSchedulePost(ctx context.Context, req *mcp.CallToolRequest, input SchedulePostInput) (*mcp.CallToolResult, SchedulePostOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, SchedulePostOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleEditor)
	if err != nil || !allowed {
		return nil, SchedulePostOutput{}, fmt.Errorf("forbidden")
	}

	scheduledAt, err := time.Parse(time.RFC3339, input.ScheduledAt)
	if err != nil {
		return nil, SchedulePostOutput{}, fmt.Errorf("invalid scheduled_at: %w", err)
	}

	createInput := domain.CreatePostInput{
		Title:                 input.Title,
		Content:               input.Content,
		ScheduledAt:           scheduledAt,
		TargetAccounts:        input.TargetAccounts,
		Visibility:            domain.NormalizePostVisibility(input.Visibility),
		AccountContentOverride: domain.NormalizeAccountContentOverride(input.AccountContentOverride, input.TargetAccounts),
	}

	post, err := h.store.CreateScheduledPost(ctx, input.TeamID, *principal, createInput)
	if err != nil {
		return nil, SchedulePostOutput{}, err
	}

	return nil, SchedulePostOutput{
		PostID:      post.ID,
		ScheduledAt: post.ScheduledAt.Format(time.RFC3339),
		Status:      string(post.Status),
	}, nil
}

// ===== Draft Post =====

func (h *Handler) handleDraftPost(ctx context.Context, req *mcp.CallToolRequest, input DraftPostInput) (*mcp.CallToolResult, DraftPostOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, DraftPostOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleEditor)
	if err != nil || !allowed {
		return nil, DraftPostOutput{}, fmt.Errorf("forbidden")
	}

	createInput := domain.CreatePostInput{
		Title:                 input.Title,
		Content:               input.Content,
		TargetAccounts:        input.TargetAccounts,
		Visibility:            domain.NormalizePostVisibility(input.Visibility),
		Draft:                 true,
		AccountContentOverride: domain.NormalizeAccountContentOverride(input.AccountContentOverride, input.TargetAccounts),
	}

	post, err := h.store.CreateScheduledPost(ctx, input.TeamID, *principal, createInput)
	if err != nil {
		return nil, DraftPostOutput{}, err
	}

	return nil, DraftPostOutput{
		PostID: post.ID,
		Status: string(post.Status),
	}, nil
}

// ===== Get Posts =====

func (h *Handler) handleGetPosts(ctx context.Context, req *mcp.CallToolRequest, input GetPostsInput) (*mcp.CallToolResult, GetPostsOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, GetPostsOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleViewer)
	if err != nil || !allowed {
		return nil, GetPostsOutput{}, fmt.Errorf("forbidden")
	}

	posts, err := h.store.ListTeamPosts(ctx, input.TeamID)
	if err != nil {
		return nil, GetPostsOutput{}, err
	}

	// Filter by status if provided
	var result []PostSummary
	for _, p := range posts {
		if input.Status != "" && string(p.Status) != input.Status {
			continue
		}
		result = append(result, PostSummary{
			ID:          p.ID,
			Title:       p.Title,
			Content:     TruncateString(p.Content, 200),
			ScheduledAt: p.ScheduledAt.Format(time.RFC3339),
			Status:      string(p.Status),
		})
	}

	return nil, GetPostsOutput{Posts: result, Total: len(result)}, nil
}

// ===== Modify Post =====

func (h *Handler) handleModifyPost(ctx context.Context, req *mcp.CallToolRequest, input ModifyPostInput) (*mcp.CallToolResult, ModifyPostOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, ModifyPostOutput{}, fmt.Errorf("unauthorized")
	}

	// Get existing post
	existing, err := h.store.GetScheduledPostByID(ctx, input.PostID)
	if err != nil {
		return nil, ModifyPostOutput{}, err
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, existing.TeamID, domain.RoleEditor)
	if err != nil || !allowed {
		return nil, ModifyPostOutput{}, fmt.Errorf("forbidden")
	}

	// Build patch
	patch := domain.UpdatePostPatch{}

	if input.Title != nil {
		patch.Title = domain.PatchField[string]{Value: *input.Title, Set: true}
	}
	if input.Content != nil {
		patch.Content = domain.PatchField[string]{Value: *input.Content, Set: true}
	}
	if input.ScheduledAt != nil {
		t, err := time.Parse(time.RFC3339, *input.ScheduledAt)
		if err != nil {
			return nil, ModifyPostOutput{}, fmt.Errorf("invalid scheduled_at: %w", err)
		}
		patch.ScheduledAt = domain.PatchField[time.Time]{Value: t, Set: true}
	}
	if input.Visibility != nil {
		v := domain.NormalizePostVisibility(*input.Visibility)
		patch.Visibility = domain.PatchField[string]{Value: v, Set: true}
	}

	// Apply patch
	_, err = h.store.PatchScheduledPost(ctx, existing.TeamID, input.PostID, patch)
	if err != nil {
		return nil, ModifyPostOutput{}, err
	}

	return nil, ModifyPostOutput{
		PostID: input.PostID,
		Status: string(existing.Status),
	}, nil
}

// ===== Delete Post =====

func (h *Handler) handleDeletePost(ctx context.Context, req *mcp.CallToolRequest, input DeletePostInput) (*mcp.CallToolResult, DeletePostOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, DeletePostOutput{}, fmt.Errorf("unauthorized")
	}

	// Get existing post
	existing, err := h.store.GetScheduledPostByID(ctx, input.PostID)
	if err != nil {
		return nil, DeletePostOutput{}, err
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, existing.TeamID, domain.RoleEditor)
	if err != nil || !allowed {
		return nil, DeletePostOutput{}, fmt.Errorf("forbidden")
	}

	err = h.store.DeleteScheduledPost(ctx, existing.TeamID, input.PostID)
	if err != nil {
		return nil, DeletePostOutput{}, err
	}

	return nil, DeletePostOutput{Success: true}, nil
}

// ===== Get Platforms =====

func (h *Handler) handleGetPlatforms(ctx context.Context, req *mcp.CallToolRequest, input GetPlatformsInput) (*mcp.CallToolResult, GetPlatformsOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, GetPlatformsOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleViewer)
	if err != nil || !allowed {
		return nil, GetPlatformsOutput{}, fmt.Errorf("forbidden")
	}

	accounts, err := h.store.ListTeamAccounts(ctx, input.TeamID)
	if err != nil {
		return nil, GetPlatformsOutput{}, err
	}

	var result []PlatformAccount
	for _, acc := range accounts {
		maxChars := domain.MaxCharsForProvider(acc.Provider, acc.MaxCharsOverride)
		result = append(result, PlatformAccount{
			AccountID: acc.ID,
			Provider:  acc.Provider,
			Username:  acc.Username,
			MaxChars:  maxChars,
		})
	}

	return nil, GetPlatformsOutput{Accounts: result}, nil
}

// ===== Get Teams =====

func (h *Handler) handleGetTeams(ctx context.Context, req *mcp.CallToolRequest, input GetTeamsInput) (*mcp.CallToolResult, GetTeamsOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, GetTeamsOutput{}, fmt.Errorf("unauthorized")
	}

	teams, err := h.store.ListTeamsForUser(ctx, principal.User.ID, principal.User.IsAdmin)
	if err != nil {
		return nil, GetTeamsOutput{}, err
	}

	var result []TeamInfo
	for _, t := range teams {
		result = append(result, TeamInfo{
			TeamID:      t.ID,
			Name:        t.Name,
			Description: t.Description,
			IsPersonal:  t.IsPersonal,
		})
	}

	return nil, GetTeamsOutput{Teams: result}, nil
}

// ===== Get Brand Profile =====

func (h *Handler) handleGetBrandProfile(ctx context.Context, req *mcp.CallToolRequest, input GetBrandProfileInput) (*mcp.CallToolResult, BrandProfileOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, BrandProfileOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleViewer)
	if err != nil || !allowed {
		return nil, BrandProfileOutput{}, fmt.Errorf("forbidden")
	}

	profile, err := h.store.GetTeamProfile(ctx, input.TeamID)
	if err != nil {
		// No profile configured
		return nil, BrandProfileOutput{HasProfile: false}, nil
	}

	output := BrandProfileOutput{
		HasProfile:      true,
		Tonality:        profile.StyleMetadata.Tonality,
		FormattingRules: profile.StyleMetadata.FormattingRules,
		BannedWords:     profile.StyleMetadata.BannedWords,
		MaxHashtags:     profile.StyleMetadata.MaxHashtags,
	}

	if profile.StyleMetadata.Identity != nil {
		output.Industry = profile.StyleMetadata.Identity.Industry
		output.TargetAudience = profile.StyleMetadata.Identity.TargetAudience
	}

	return nil, output, nil
}

// ===== Search Posts =====

func (h *Handler) handleSearchPosts(ctx context.Context, req *mcp.CallToolRequest, input SearchPostsInput) (*mcp.CallToolResult, SearchPostsOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, SearchPostsOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleViewer)
	if err != nil || !allowed {
		return nil, SearchPostsOutput{}, fmt.Errorf("forbidden")
	}

	posts, err := h.store.ListTeamPosts(ctx, input.TeamID)
	if err != nil {
		return nil, SearchPostsOutput{}, err
	}

	// Parse date range
	var from, to time.Time
	if input.FromDate != "" {
		from, _ = time.Parse(time.RFC3339, input.FromDate)
	}
	if input.ToDate != "" {
		to, _ = time.Parse(time.RFC3339, input.ToDate)
	}

	query := strings.ToLower(strings.TrimSpace(input.Query))
	var result []PostSummary
	for _, p := range posts {
		// Filter by status
		if input.Status != "" && string(p.Status) != input.Status {
			continue
		}

		// Filter by date range
		if !from.IsZero() && p.ScheduledAt.Before(from) {
			continue
		}
		if !to.IsZero() && p.ScheduledAt.After(to) {
			continue
		}

		// Filter by query (match in title or content)
		if query != "" {
			titleMatch := strings.Contains(strings.ToLower(p.Title), query)
			contentMatch := strings.Contains(strings.ToLower(p.Content), query)
			if !titleMatch && !contentMatch {
				continue
			}
		}

		result = append(result, PostSummary{
			ID:          p.ID,
			Title:       p.Title,
			Content:     TruncateString(p.Content, 200),
			ScheduledAt: p.ScheduledAt.Format(time.RFC3339),
			Status:      string(p.Status),
		})
	}

	return nil, SearchPostsOutput{Posts: result, Total: len(result)}, nil
}

// ===== Get Analytics =====

func (h *Handler) handleGetAnalytics(ctx context.Context, req *mcp.CallToolRequest, input GetAnalyticsInput) (*mcp.CallToolResult, GetAnalyticsOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, GetAnalyticsOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleViewer)
	if err != nil || !allowed {
		return nil, GetAnalyticsOutput{}, fmt.Errorf("forbidden")
	}

	analytics, err := h.store.GetTeamAnalytics(ctx, input.TeamID, 10)
	if err != nil {
		return nil, GetAnalyticsOutput{}, err
	}

	var metrics []MetricValue
	for metric, total := range analytics.MetricsTotal {
		metrics = append(metrics, MetricValue{
			Metric: metric,
			Total:  total,
		})
	}

	var topPosts []PostSummary
	for _, p := range analytics.TopPosts {
		topPosts = append(topPosts, PostSummary{
			ID:   p.PostID,
			Title: p.Title,
		})
	}

	return nil, GetAnalyticsOutput{
		Metrics:  metrics,
		TopPosts: topPosts,
	}, nil
}

// ===== Get Hashtag Performance =====

func (h *Handler) handleGetHashtagPerformance(ctx context.Context, req *mcp.CallToolRequest, input GetHashtagPerformanceInput) (*mcp.CallToolResult, GetHashtagPerformanceOutput, error) {
	principal := principalFromContext(ctx)
	if principal == nil {
		return nil, GetHashtagPerformanceOutput{}, fmt.Errorf("unauthorized")
	}

	allowed, err := h.auth.PrincipalHasTeamAccess(ctx, *principal, input.TeamID, domain.RoleViewer)
	if err != nil || !allowed {
		return nil, GetHashtagPerformanceOutput{}, fmt.Errorf("forbidden")
	}

	days := input.Days
	if days <= 0 {
		days = 90
	}
	limit := input.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	items, err := h.store.ListTeamHashtagPerformance(ctx, input.TeamID, days, input.Provider, limit)
	if err != nil {
		return nil, GetHashtagPerformanceOutput{}, err
	}

	out := GetHashtagPerformanceOutput{Hashtags: make([]HashtagPerformanceValue, 0, len(items))}
	for _, item := range items {
		out.Hashtags = append(out.Hashtags, HashtagPerformanceValue{
			Tag:             item.Tag,
			Display:         item.Display,
			Uses:            item.Uses,
			TotalEngagement: item.TotalEngagement,
			AvgEngagement:   item.AvgEngagement,
			Score:           item.Score,
		})
	}
	return nil, out, nil
}
