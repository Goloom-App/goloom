package mcp

// ===== Campaigns =====

type CreateCampaignInput struct {
	TeamID           string   `json:"team_id" jsonschema:"required,description=Team ID"`
	Name             string   `json:"name" jsonschema:"required,description=Campaign name"`
	Structure        string   `json:"structure" jsonschema:"required,description=Campaign structure as JSON string"`
	RequiredHashtags []string `json:"required_hashtags,omitempty" jsonschema:"description=Hashtags to include in posts"`
}

type CreateCampaignOutput struct {
	CampaignID string `json:"campaign_id"`
	Name       string `json:"name"`
}

type GetCampaignInput struct {
	TeamID     string `json:"team_id" jsonschema:"required,description=Team ID"`
	CampaignID string `json:"campaign_id" jsonschema:"required,description=Campaign format ID"`
}

type GetCampaignOutput struct {
	CampaignID       string   `json:"campaign_id"`
	Name             string   `json:"name"`
	Weekday          *int     `json:"weekday,omitempty"`
	Structure        string   `json:"structure"`
	RequiredHashtags []string `json:"required_hashtags"`
	IsActive         bool     `json:"is_active"`
}

// ===== Recurring Posts =====

type CreateRecurringInput struct {
	TeamID           string            `json:"team_id" jsonschema:"required,description=Team ID"`
	Title            string            `json:"title" jsonschema:"required,description=Post title with optional {{variables}}"`
	Content          string            `json:"content" jsonschema:"required,description=Post content with optional {{variables}}"`
	RecurrenceJSON   string            `json:"recurrence_json" jsonschema:"required,description=RRULE string (e.g., FREQ=WEEKLY;BYDAY=TU)"`
	TargetAccounts   []string          `json:"target_accounts" jsonschema:"required,description=List of account IDs"`
	Visibility       string            `json:"visibility,omitempty" jsonschema:"description=public,unlisted,private,direct"`
	Enabled          *bool             `json:"enabled,omitempty" jsonschema:"description=Enable immediately (default: true)"`
	AccountContentOverride map[string]string `json:"account_content_override,omitempty" jsonschema:"description=Per-account content overrides"`
}

type CreateRecurringOutput struct {
	TemplateID string `json:"template_id"`
}

// ===== RSS Feeds =====

type CreateRSSFeedInput struct {
	TeamID           string   `json:"team_id" jsonschema:"required,description=Team ID"`
	FeedURL          string   `json:"feed_url" jsonschema:"required,description=RSS feed URL"`
	Name             string   `json:"name" jsonschema:"required,description=Feed name"`
	ContentTemplate  string   `json:"content_template,omitempty" jsonschema:"description=Content template with {title}, {link}, {description}"`
	TargetAccountIDs []string `json:"target_account_ids" jsonschema:"required,description=List of account IDs"`
	OutputMode       string   `json:"output_mode,omitempty" jsonschema:"description=draft,scheduled,publish_now"`
}

type CreateRSSFeedOutput struct {
	FeedID string `json:"feed_id"`
}

// ===== Calendar =====

type GetCalendarInput struct {
	TeamID   string `json:"team_id" jsonschema:"required,description=Team ID"`
	FromDate string `json:"from_date,omitempty" jsonschema:"description=Start date (RFC3339, default: -7 days)"`
	ToDate   string `json:"to_date,omitempty" jsonschema:"description=End date (RFC3339, default: +7 days)"`
}

type CalendarPost struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Content     string `json:"content"`
	ScheduledAt string `json:"scheduled_at"`
	Status      string `json:"status"`
}

type GetCalendarOutput struct {
	Posts []CalendarPost `json:"posts"`
	Total int            `json:"total"`
}

// ===== Free Slot Finding =====

type FindFreeSlotInput struct {
	TeamID     string `json:"team_id" jsonschema:"required,description=Team ID"`
	Weekday    string `json:"weekday,omitempty" jsonschema:"description=monday,tuesday,wednesday,thursday,friday,saturday,sunday"`
	AfterDate  string `json:"after_date,omitempty" jsonschema:"description=Start searching from (RFC3339, default: now)"`
	BeforeDate string `json:"before_date,omitempty" jsonschema:"description=Stop searching at (RFC3339, default: +30 days)"`
}

type FindFreeSlotOutput struct {
	Date      string `json:"date,omitempty"`
	Weekday   string `json:"weekday,omitempty"`
	Available bool   `json:"available"`
}

// ===== Schedule Post =====

type SchedulePostInput struct {
	TeamID        string            `json:"team_id" jsonschema:"required,description=Team ID"`
	Title         string            `json:"title,omitempty" jsonschema:"description=Post title"`
	Content       string            `json:"content" jsonschema:"required,description=Post content"`
	ScheduledAt   string            `json:"scheduled_at" jsonschema:"required,description=Schedule datetime (RFC3339)"`
	TargetAccounts []string         `json:"target_accounts" jsonschema:"required,description=List of account IDs"`
	Visibility    string            `json:"visibility,omitempty" jsonschema:"description=public,unlisted,private,direct"`
	AccountContentOverride map[string]string `json:"account_content_override,omitempty" jsonschema:"description=Per-account content for character limits"`
}

type SchedulePostOutput struct {
	PostID      string `json:"post_id"`
	ScheduledAt string `json:"scheduled_at"`
	Status      string `json:"status"`
}

// ===== Draft Post =====

type DraftPostInput struct {
	TeamID        string            `json:"team_id" jsonschema:"required,description=Team ID"`
	Title         string            `json:"title,omitempty" jsonschema:"description=Post title"`
	Content       string            `json:"content" jsonschema:"required,description=Post content"`
	TargetAccounts []string         `json:"target_accounts" jsonschema:"required,description=List of account IDs"`
	Visibility    string            `json:"visibility,omitempty" jsonschema:"description=public,unlisted,private,direct"`
	AccountContentOverride map[string]string `json:"account_content_override,omitempty" jsonschema:"description=Per-account content for character limits"`
}

type DraftPostOutput struct {
	PostID string `json:"post_id"`
	Status string `json:"status"`
}

// ===== Get Posts =====

type GetPostsInput struct {
	TeamID string `json:"team_id" jsonschema:"required,description=Team ID"`
	Status string `json:"status,omitempty" jsonschema:"description=pending,processing,posted,failed,cancelled,draft"`
}

type PostSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Content     string `json:"content"`
	ScheduledAt string `json:"scheduled_at"`
	Status      string `json:"status"`
}

type GetPostsOutput struct {
	Posts []PostSummary `json:"posts"`
	Total int           `json:"total"`
}

// ===== Modify Post =====

type ModifyPostInput struct {
	TeamID        string            `json:"team_id" jsonschema:"required,description=Team ID"`
	PostID        string            `json:"post_id" jsonschema:"required,description=Post ID to modify"`
	Title         *string           `json:"title,omitempty" jsonschema:"description=New title (null to keep current)"`
	Content       *string           `json:"content,omitempty" jsonschema:"description=New content (null to keep current)"`
	ScheduledAt   *string           `json:"scheduled_at,omitempty" jsonschema:"description=New schedule (RFC3339, null to keep)"`
	TargetAccounts *[]string        `json:"target_accounts,omitempty" jsonschema:"description=New target accounts (null to keep)"`
	Visibility    *string           `json:"visibility,omitempty" jsonschema:"description=New visibility (null to keep)"`
	AccountContentOverride map[string]string `json:"account_content_override,omitempty" jsonschema:"description=Per-account content overrides"`
}

type ModifyPostOutput struct {
	PostID string `json:"post_id"`
	Status string `json:"status"`
}

// ===== Delete Post =====

type DeletePostInput struct {
	TeamID string `json:"team_id" jsonschema:"required,description=Team ID"`
	PostID string `json:"post_id" jsonschema:"required,description=Post ID to delete"`
}

type DeletePostOutput struct {
	Success bool `json:"success"`
}

// ===== Get Platforms =====

type GetPlatformsInput struct {
	TeamID string `json:"team_id" jsonschema:"required,description=Team ID"`
}

type PlatformAccount struct {
	AccountID string `json:"account_id"`
	Provider  string `json:"provider"`
	Username  string `json:"username"`
	MaxChars  int    `json:"max_chars"`
}

type GetPlatformsOutput struct {
	Accounts []PlatformAccount `json:"accounts"`
}

// ===== Get Teams =====

type GetTeamsInput struct{}

type TeamInfo struct {
	TeamID      string `json:"team_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsPersonal  bool   `json:"is_personal"`
}

type GetTeamsOutput struct {
	Teams []TeamInfo `json:"teams"`
}

// ===== Get Brand Profile =====

type GetBrandProfileInput struct {
	TeamID string `json:"team_id" jsonschema:"required,description=Team ID"`
}

type BrandProfileOutput struct {
	HasProfile      bool   `json:"has_profile"`
	Tonality        string `json:"tonality,omitempty"`
	FormattingRules []string `json:"formatting_rules,omitempty"`
	BannedWords     []string `json:"banned_words,omitempty"`
	MaxHashtags     int    `json:"max_hashtags,omitempty"`
	Industry        string `json:"industry,omitempty"`
	TargetAudience  string `json:"target_audience,omitempty"`
}

// ===== Search Posts =====

type SearchPostsInput struct {
	TeamID    string `json:"team_id" jsonschema:"required,description=Team ID"`
	Query     string `json:"query" jsonschema:"required,description=Search query (matches content)"`
	FromDate  string `json:"from_date,omitempty" jsonschema:"description=Start date (RFC3339)"`
	ToDate    string `json:"to_date,omitempty" jsonschema:"description=End date (RFC3339)"`
	Status    string `json:"status,omitempty" jsonschema:"description=Filter by status"`
}

type SearchPostsOutput struct {
	Posts []PostSummary `json:"posts"`
	Total int           `json:"total"`
}

// ===== Get Analytics =====

type GetAnalyticsInput struct {
	TeamID string `json:"team_id" jsonschema:"required,description=Team ID"`
}

type MetricValue struct {
	Metric string `json:"metric"`
	Total  int64  `json:"total"`
}

type GetAnalyticsOutput struct {
	Metrics    []MetricValue     `json:"metrics"`
	TopPosts   []PostSummary     `json:"top_posts,omitempty"`
}

// ===== Get Hashtag Performance =====

type GetHashtagPerformanceInput struct {
	TeamID   string `json:"team_id" jsonschema:"required,description=Team ID"`
	Days     int    `json:"days,omitempty" jsonschema:"description=Time window in days (default 90; max 366)"`
	Provider string `json:"provider,omitempty" jsonschema:"description=Filter by platform: bluesky; mastodon or friendica. Omit for all platforms"`
	Limit    int    `json:"limit,omitempty" jsonschema:"description=Maximum number of hashtags (default 20; max 50)"`
}

type HashtagPerformanceValue struct {
	Tag             string  `json:"tag"`
	Display         string  `json:"display"`
	Uses            int64   `json:"uses"`
	TotalEngagement int64   `json:"total_engagement"`
	AvgEngagement   float64 `json:"avg_engagement"`
	Score           float64 `json:"score"`
}

type GetHashtagPerformanceOutput struct {
	Hashtags []HashtagPerformanceValue `json:"hashtags"`
}
