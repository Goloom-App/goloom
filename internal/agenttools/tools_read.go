package agenttools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// roleViewer is the role set every read tool requires.
var roleViewer = []domain.TeamRole{domain.RoleViewer, domain.RoleEditor, domain.RoleOwner}

func coreGetCampaign(ctx context.Context, d Deps, inv Invocation, in GetCampaignInput) (GetCampaignOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return GetCampaignOutput{}, err
	}
	campaign, err := d.Store.GetCampaignFormatByID(ctx, in.TeamID, in.CampaignID)
	if err != nil {
		return GetCampaignOutput{}, err
	}
	return GetCampaignOutput{
		CampaignID:       campaign.ID,
		Name:             campaign.Name,
		Weekday:          campaign.Weekday,
		Structure:        string(campaign.Structure),
		RequiredHashtags: campaign.RequiredHashtags,
		IsActive:         campaign.IsActive,
	}, nil
}

func coreGetCalendar(ctx context.Context, d Deps, inv Invocation, in GetCalendarInput) (GetCalendarOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return GetCalendarOutput{}, err
	}
	posts, err := d.Store.ListTeamPosts(ctx, in.TeamID)
	if err != nil {
		return GetCalendarOutput{}, err
	}

	from := time.Now().AddDate(0, 0, -7)
	to := time.Now().AddDate(0, 0, 7)
	if in.FromDate != "" {
		if t, err := time.Parse(time.RFC3339, in.FromDate); err == nil {
			from = t
		}
	}
	if in.ToDate != "" {
		if t, err := time.Parse(time.RFC3339, in.ToDate); err == nil {
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
	return GetCalendarOutput{Posts: result, Total: len(result)}, nil
}

func coreFindFreeSlot(ctx context.Context, d Deps, inv Invocation, in FindFreeSlotInput) (FindFreeSlotOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return FindFreeSlotOutput{}, err
	}

	after := time.Now().UTC()
	if in.AfterDate != "" {
		if t, err := time.Parse(time.RFC3339, in.AfterDate); err == nil {
			after = t
		}
	}
	before := after.AddDate(0, 0, 30)
	if in.BeforeDate != "" {
		if t, err := time.Parse(time.RFC3339, in.BeforeDate); err == nil {
			before = t
		}
	}

	targetWeekday := ParseWeekday(in.Weekday)

	posts, err := d.Store.ListTeamPosts(ctx, in.TeamID)
	if err != nil {
		return FindFreeSlotOutput{}, err
	}

	date, found := FindNextFreeSlot(posts, after, before, targetWeekday)
	if !found {
		return FindFreeSlotOutput{Available: false}, nil
	}
	return FindFreeSlotOutput{
		Date:      date.Format(time.RFC3339),
		Weekday:   date.Weekday().String(),
		Available: true,
	}, nil
}

func coreGetPosts(ctx context.Context, d Deps, inv Invocation, in GetPostsInput) (GetPostsOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return GetPostsOutput{}, err
	}
	posts, err := d.Store.ListTeamPosts(ctx, in.TeamID)
	if err != nil {
		return GetPostsOutput{}, err
	}
	var result []PostSummary
	for _, p := range posts {
		if in.Status != "" && string(p.Status) != in.Status {
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
	return GetPostsOutput{Posts: result, Total: len(result)}, nil
}

func coreSearchPosts(ctx context.Context, d Deps, inv Invocation, in SearchPostsInput) (SearchPostsOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return SearchPostsOutput{}, err
	}
	posts, err := d.Store.ListTeamPosts(ctx, in.TeamID)
	if err != nil {
		return SearchPostsOutput{}, err
	}

	var from, to time.Time
	if in.FromDate != "" {
		from, _ = time.Parse(time.RFC3339, in.FromDate)
	}
	if in.ToDate != "" {
		to, _ = time.Parse(time.RFC3339, in.ToDate)
	}

	query := strings.ToLower(strings.TrimSpace(in.Query))
	var result []PostSummary
	for _, p := range posts {
		if in.Status != "" && string(p.Status) != in.Status {
			continue
		}
		if !from.IsZero() && p.ScheduledAt.Before(from) {
			continue
		}
		if !to.IsZero() && p.ScheduledAt.After(to) {
			continue
		}
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
	return SearchPostsOutput{Posts: result, Total: len(result)}, nil
}

func coreGetPlatforms(ctx context.Context, d Deps, inv Invocation, in GetPlatformsInput) (GetPlatformsOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return GetPlatformsOutput{}, err
	}
	accounts, err := d.Store.ListTeamAccounts(ctx, in.TeamID)
	if err != nil {
		return GetPlatformsOutput{}, err
	}
	var result []PlatformAccount
	for _, acc := range accounts {
		result = append(result, PlatformAccount{
			AccountID: acc.ID,
			Provider:  acc.Provider,
			Username:  acc.Username,
			MaxChars:  domain.MaxCharsForProvider(acc.Provider, acc.MaxCharsOverride),
		})
	}
	return GetPlatformsOutput{Accounts: result}, nil
}

func coreGetTeams(ctx context.Context, d Deps, inv Invocation, _ GetTeamsInput) (GetTeamsOutput, error) {
	teams, err := d.Store.ListTeamsForUser(ctx, inv.Principal.User.ID, inv.Principal.User.IsAdmin)
	if err != nil {
		return GetTeamsOutput{}, err
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
	return GetTeamsOutput{Teams: result}, nil
}

func coreGetBrandProfile(ctx context.Context, d Deps, inv Invocation, in GetBrandProfileInput) (BrandProfileOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return BrandProfileOutput{}, err
	}
	profile, err := d.Store.GetTeamProfile(ctx, in.TeamID)
	if err != nil {
		// No profile configured.
		return BrandProfileOutput{HasProfile: false}, nil
	}
	out := BrandProfileOutput{
		HasProfile:      true,
		Tonality:        profile.StyleMetadata.Tonality,
		FormattingRules: profile.StyleMetadata.FormattingRules,
		BannedWords:     profile.StyleMetadata.BannedWords,
		MaxHashtags:     profile.StyleMetadata.MaxHashtags,
	}
	if profile.StyleMetadata.Identity != nil {
		out.Industry = profile.StyleMetadata.Identity.Industry
		out.TargetAudience = profile.StyleMetadata.Identity.TargetAudience
	}
	return out, nil
}

func coreGetAnalytics(ctx context.Context, d Deps, inv Invocation, in GetAnalyticsInput) (GetAnalyticsOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return GetAnalyticsOutput{}, err
	}
	analytics, err := d.Store.GetTeamAnalytics(ctx, in.TeamID, 10)
	if err != nil {
		return GetAnalyticsOutput{}, err
	}
	var metrics []MetricValue
	for metric, total := range analytics.MetricsTotal {
		metrics = append(metrics, MetricValue{Metric: metric, Total: total})
	}
	var topPosts []PostSummary
	for _, p := range analytics.TopPosts {
		topPosts = append(topPosts, PostSummary{ID: p.PostID, Title: p.Title})
	}
	return GetAnalyticsOutput{Metrics: metrics, TopPosts: topPosts}, nil
}

func coreGetHashtagPerformance(ctx context.Context, d Deps, inv Invocation, in GetHashtagPerformanceInput) (GetHashtagPerformanceOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return GetHashtagPerformanceOutput{}, err
	}
	days := in.Days
	if days <= 0 {
		days = 90
	}
	limit := in.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	items, err := d.Store.ListTeamHashtagPerformance(ctx, in.TeamID, days, in.Provider, limit)
	if err != nil {
		return GetHashtagPerformanceOutput{}, err
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
	return out, nil
}

func coreGetAnalyticsTimeslots(ctx context.Context, d Deps, inv Invocation, in GetAnalyticsTimeslotsInput) (GetAnalyticsTimeslotsOutput, error) {
	if err := requireTeam(ctx, d, inv, in.TeamID, roleViewer...); err != nil {
		return GetAnalyticsTimeslotsOutput{}, err
	}
	loc := time.UTC
	if tz := strings.TrimSpace(in.Timezone); tz != "" {
		parsed, err := time.LoadLocation(tz)
		if err != nil {
			return GetAnalyticsTimeslotsOutput{}, fmt.Errorf("invalid timezone %q: %w", tz, err)
		}
		loc = parsed
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 50 {
		limit = 50
	}
	posts, err := d.Store.ListTeamPostEngagement(ctx, in.TeamID, in.Days, in.Provider)
	if err != nil {
		return GetAnalyticsTimeslotsOutput{}, err
	}
	slots := domain.AggregateTimeslots(posts, loc, limit)
	out := GetAnalyticsTimeslotsOutput{
		Timezone:  loc.String(),
		Timeslots: make([]TimeslotValue, 0, len(slots)),
	}
	for _, slot := range slots {
		out.Timeslots = append(out.Timeslots, TimeslotValue{
			Weekday:         slot.Weekday.String(),
			Hour:            slot.Hour,
			Posts:           slot.Posts,
			TotalEngagement: slot.TotalEngagement,
			AvgEngagement:   slot.AvgEngagement,
			Score:           slot.Score,
		})
	}
	return out, nil
}
