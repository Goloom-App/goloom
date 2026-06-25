package agenttools

import "git.f4mily.net/goloom/internal/auth"

// All returns the full agent tool catalog. Each tool is defined exactly once and
// tagged with the transports that expose it: read tools and brand recall are
// shared by the MCP server and the in-app chat assistant; write tools are
// currently MCP-only (the chat assistant gains them, behind a confirmation flow,
// in a later step). get_teams is MCP-only because the chat is already scoped to
// one team via its request path.
func All() []*Tool {
	return []*Tool{
		// ===== Reads (shared MCP + chat) =====
		define[GetCalendarInput, GetCalendarOutput](spec{
			name:       "get_calendar",
			desc:       "Get posts for a date range. Returns scheduled, pending, and draft posts.",
			transports: transportsShared,
		}, coreGetCalendar),
		define[FindFreeSlotInput, FindFreeSlotOutput](spec{
			name:       "find_free_slot",
			desc:       "Find the next available day with no scheduled post. Supports weekday names (tuesday, friday).",
			transports: transportsShared,
		}, coreFindFreeSlot),
		define[GetPostsInput, GetPostsOutput](spec{
			name:       "get_posts",
			desc:       "List the team's posts with an optional status filter.",
			transports: transportsShared,
		}, coreGetPosts),
		define[SearchPostsInput, SearchPostsOutput](spec{
			name:       "search_posts",
			desc:       "Search posts by content text, date range, or status.",
			transports: transportsShared,
		}, coreSearchPosts),
		define[GetCampaignInput, GetCampaignOutput](spec{
			name:       "get_campaign",
			desc:       "Get campaign details including full structure, hashtags, and weekday.",
			transports: transportsShared,
		}, coreGetCampaign),
		define[GetPlatformsInput, GetPlatformsOutput](spec{
			name:       "get_platforms",
			desc:       "List connected accounts with provider, username, and character limits.",
			transports: transportsShared,
		}, coreGetPlatforms),
		define[GetBrandProfileInput, BrandProfileOutput](spec{
			name:       "get_brand_profile",
			desc:       "Recall the team's brand profile (tonality, formatting rules, banned words, industry, audience). Call this before writing post content so the copy matches the team's voice.",
			transports: transportsShared,
		}, coreGetBrandProfile),
		define[GetAnalyticsInput, GetAnalyticsOutput](spec{
			name:       "get_analytics",
			desc:       "Get engagement analytics (likes, reposts, followers) for the team plus its top posts.",
			transports: transportsShared,
		}, coreGetAnalytics),
		define[GetHashtagPerformanceInput, GetHashtagPerformanceOutput](spec{
			name:       "get_hashtag_performance",
			desc:       "Get the team's best-performing hashtags from published post analytics (uses, engagement, smoothed score), optionally filtered by platform and time window.",
			transports: transportsShared,
		}, coreGetHashtagPerformance),
		define[GetAnalyticsTimeslotsInput, GetAnalyticsTimeslotsOutput](spec{
			name:       "get_analytics_timeslots",
			desc:       "Find the best times to post: ranks weekday/hour slots by historical engagement. Supports an IANA timezone (e.g. Europe/Berlin) and optional platform/time-window filters.",
			transports: transportsShared,
		}, coreGetAnalyticsTimeslots),

		// ===== Reads (chat only) =====
		define[GetCurrentViewInput, GetCurrentViewOutput](spec{
			name:       "get_current_view",
			desc:       "See what the user is currently looking at in the app (active section, focused entity, visible data). Call this to ground your help in their current screen, e.g. when they say 'this post' or 'here'.",
			transports: transportsChatOnly,
		}, coreGetCurrentView),

		// ===== Reads (MCP only) =====
		define[GetTeamsInput, GetTeamsOutput](spec{
			name:       "get_teams",
			desc:       "List teams the authenticated user has access to.",
			transports: transportsMCPOnly,
		}, coreGetTeams),

		// ===== Writes =====
		// Autonomous (no publish): drafts, in-place edits and campaign-format
		// definitions run immediately. Confirm=true actions (scheduling,
		// deletion, auto-publishing automations) are only proposed in the chat and
		// run after the user confirms; the MCP adapter ignores Confirm.
		define[DraftPostInput, DraftPostOutput](spec{
			name:       "draft_post",
			desc:       "Save a post as a draft (not scheduled, not published). Pass target_account_ids; use account_content_override for shorter per-platform variants.",
			scope:      auth.ScopeWriteDraft,
			transports: transportsShared,
		}, coreDraftPost),
		define[ModifyPostInput, ModifyPostOutput](spec{
			name:       "modify_post",
			desc:       "Update an existing draft or scheduled post (content, schedule, targets, per-account overrides). Use this for changes to a post that already exists — never create a second draft.",
			scope:      auth.ScopeWrite,
			transports: transportsShared,
		}, coreModifyPost),
		define[CreateCampaignInput, CreateCampaignOutput](spec{
			name:       "create_campaign",
			desc:       "Create a campaign format (a reusable post-series definition). Does not publish anything.",
			scope:      auth.ScopeWrite,
			transports: transportsShared,
		}, coreCreateCampaign),
		define[SchedulePostInput, SchedulePostOutput](spec{
			name:       "schedule_post",
			desc:       "Schedule a post for publication at a specific time. Use account_content_override for per-platform character limits.",
			scope:      auth.ScopeWriteSchedule,
			confirm:    true,
			transports: transportsShared,
		}, coreSchedulePost),
		define[DeletePostInput, DeletePostOutput](spec{
			name:       "delete_post",
			desc:       "Delete a scheduled or draft post.",
			scope:      auth.ScopeDelete,
			confirm:    true,
			transports: transportsShared,
		}, coreDeletePost),
		define[CreateRecurringInput, CreateRecurringOutput](spec{
			name:       "create_recurring",
			desc:       "Create a recurring post automation that auto-publishes on an RRULE schedule.",
			scope:      auth.ScopeWrite,
			confirm:    true,
			transports: transportsShared,
		}, coreCreateRecurring),
		define[CreateRSSFeedInput, CreateRSSFeedOutput](spec{
			name:       "create_rss_feed",
			desc:       "Create an RSS feed automation that turns new feed items into posts.",
			scope:      auth.ScopeWrite,
			confirm:    true,
			transports: transportsShared,
		}, coreCreateRSSFeed),
	}
}
