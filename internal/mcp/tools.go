package mcp

import "github.com/modelcontextprotocol/go-sdk/mcp"

// registerTools registers all MCP tools with the server.
func (h *Handler) registerTools(server *mcp.Server) {
	// === Campaigns ===
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_campaign",
		Description: "Create a new campaign with structure, hashtags, and optional instructions for AI agents",
	}, h.handleCreateCampaign)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_campaign",
		Description: "Get campaign details including full instructions, structure, and hashtags",
	}, h.handleGetCampaign)

	// === Recurring Posts ===
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_recurring",
		Description: "Create a recurring post template with RRULE schedule",
	}, h.handleCreateRecurring)

	// === RSS Feeds ===
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_rss_feed",
		Description: "Create an RSS feed automation with content template",
	}, h.handleCreateRSSFeed)

	// === Calendar & Scheduling ===
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_calendar",
		Description: "Get posts for a date range. Returns scheduled, pending, and draft posts.",
	}, h.handleGetCalendar)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "find_free_slot",
		Description: "Find next available time slot. Supports weekday names (tuesday, friday) or 'next_free_tuesday'.",
	}, h.handleFindFreeSlot)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "schedule_post",
		Description: "Schedule a post. Use account_content_override for per-platform character limits.",
	}, h.handleSchedulePost)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "draft_post",
		Description: "Save a post as draft (not scheduled, not published)",
	}, h.handleDraftPost)

	// === Post Management ===
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_posts",
		Description: "List posts with optional status filter",
	}, h.handleGetPosts)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "modify_post",
		Description: "Update an existing post (content, schedule, targets, overrides)",
	}, h.handleModifyPost)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_post",
		Description: "Delete a scheduled or draft post",
	}, h.handleDeletePost)

	// === Metadata ===
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_platforms",
		Description: "List connected accounts with provider, username, and character limits",
	}, h.handleGetPlatforms)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_teams",
		Description: "List teams the authenticated user has access to",
	}, h.handleGetTeams)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_brand_profile",
		Description: "Get brand profile (tonality, style rules, identity, knowledge sources)",
	}, h.handleGetBrandProfile)

	// === Search & Analytics ===
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_posts",
		Description: "Search posts by content text, date range, or status",
	}, h.handleSearchPosts)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_analytics",
		Description: "Get engagement analytics (likes, reposts, followers) for a team or specific posts",
	}, h.handleGetAnalytics)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_hashtag_performance",
		Description: "Get the team's best-performing hashtags from published post analytics (uses, engagement, smoothed score), optionally filtered by platform and time window",
	}, h.handleGetHashtagPerformance)
}
