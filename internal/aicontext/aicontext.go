// Package aicontext composes the domain.AIContext handed to prompt builders.
// It is the single place that assembles brand data, accounts, posts and
// analytics from store primitives; callers choose the parts they need.
package aicontext

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

const (
	analyticsWindowDays = 90
	topHashtagsLimit    = 20
)

// Source is the subset of store.Store the builder needs.
type Source interface {
	GetTeamByID(ctx context.Context, teamID string) (domain.Team, error)
	GetTeamProfile(ctx context.Context, teamID string) (domain.TeamProfile, error)
	ListCampaignFormats(ctx context.Context, teamID string) ([]domain.CampaignFormat, error)
	ListStyleExamples(ctx context.Context, teamID string) ([]domain.StyleExample, error)
	ListKnowledgeSources(ctx context.Context, teamID string) ([]domain.KnowledgeSource, error)
	ListTeamAccounts(ctx context.Context, teamID string) ([]domain.SocialAccount, error)
	ListRecentPostedPosts(ctx context.Context, teamID string, limit int) ([]domain.ScheduledPost, error)
	ListUpcomingPosts(ctx context.Context, teamID string, from time.Time, limit int) ([]domain.ScheduledPost, error)
	GetTeamEngagementHourHistogram(ctx context.Context, teamID string, days int) ([]domain.EngagementHourBucket, error)
	ListTeamHashtagPerformance(ctx context.Context, teamID string, days int, provider string, limit int) ([]domain.HashtagPerformance, error)
}

// Options selects the optional parts of the context. Team, brand profile,
// style examples, knowledge sources, campaign formats and accounts are always
// loaded.
type Options struct {
	Posts      bool // recent posted + upcoming posts
	Engagement bool // engagement-by-hour histogram
	Hashtags   bool // top performing hashtags
}

// Full loads everything (generation jobs, prompt preview, GET /ai-context).
func Full() Options { return Options{Posts: true, Engagement: true, Hashtags: true} }

// Chat loads what the chat system prompt uses: brand, accounts, formats and
// top hashtags, but no posts or engagement histogram.
func Chat() Options { return Options{Hashtags: true} }

// Build assembles the AI context for a team. Slices are never nil and a
// missing brand profile yields a nil Profile without error, so prompt code can
// fall back to defaults.
func Build(ctx context.Context, src Source, teamID string, opts Options) (domain.AIContext, error) {
	team, err := src.GetTeamByID(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, fmt.Errorf("aicontext: get team: %w", err)
	}
	out := domain.AIContext{
		Team:             team,
		CampaignFormats:  []domain.CampaignFormat{},
		StyleExamples:    []domain.StyleExample{},
		KnowledgeSources: []domain.KnowledgeSource{},
		RecentPosts:      []domain.ScheduledPost{},
		Accounts:         []domain.AIAccountSummary{},
		UpcomingPosts:    []domain.ScheduledPost{},
		EngagementHours:  []domain.EngagementHourBucket{},
		TopHashtags:      []domain.HashtagPerformance{},
	}

	profile, err := src.GetTeamProfile(ctx, teamID)
	switch {
	case err == nil:
		out.Profile = &profile
	case errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows):
		// no brand profile: leave nil
	default:
		return domain.AIContext{}, fmt.Errorf("aicontext: get profile: %w", err)
	}

	if v, err := src.ListCampaignFormats(ctx, teamID); err != nil {
		return domain.AIContext{}, fmt.Errorf("aicontext: list formats: %w", err)
	} else if v != nil {
		out.CampaignFormats = v
	}
	if v, err := src.ListStyleExamples(ctx, teamID); err != nil {
		return domain.AIContext{}, fmt.Errorf("aicontext: list style examples: %w", err)
	} else if v != nil {
		out.StyleExamples = v
	}
	if v, err := src.ListKnowledgeSources(ctx, teamID); err != nil {
		return domain.AIContext{}, fmt.Errorf("aicontext: list knowledge sources: %w", err)
	} else if v != nil {
		out.KnowledgeSources = v
	}

	accounts, err := src.ListTeamAccounts(ctx, teamID)
	if err != nil {
		return domain.AIContext{}, fmt.Errorf("aicontext: list accounts: %w", err)
	}
	for _, acc := range accounts {
		out.Accounts = append(out.Accounts, domain.AIAccountSummary{
			ID:       acc.ID,
			Provider: acc.Provider,
			Username: acc.Username,
			MaxChars: domain.MaxCharsForProvider(acc.Provider, acc.MaxCharsOverride),
		})
	}

	if opts.Posts {
		if v, err := src.ListRecentPostedPosts(ctx, teamID, domain.AIContextRecentPostsLimit); err != nil {
			return domain.AIContext{}, fmt.Errorf("aicontext: list recent posts: %w", err)
		} else if v != nil {
			out.RecentPosts = v
		}
		if v, err := src.ListUpcomingPosts(ctx, teamID, time.Now().UTC(), domain.AIContextUpcomingPostsLimit); err != nil {
			return domain.AIContext{}, fmt.Errorf("aicontext: list upcoming posts: %w", err)
		} else if v != nil {
			out.UpcomingPosts = v
		}
	}
	if opts.Engagement {
		if v, err := src.GetTeamEngagementHourHistogram(ctx, teamID, analyticsWindowDays); err != nil {
			return domain.AIContext{}, fmt.Errorf("aicontext: engagement hours: %w", err)
		} else if v != nil {
			out.EngagementHours = v
		}
	}
	if opts.Hashtags {
		if v, err := src.ListTeamHashtagPerformance(ctx, teamID, analyticsWindowDays, "", topHashtagsLimit); err != nil {
			return domain.AIContext{}, fmt.Errorf("aicontext: top hashtags: %w", err)
		} else if v != nil {
			out.TopHashtags = v
		}
	}
	return out, nil
}
