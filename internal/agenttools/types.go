package agenttools

// ===== Campaigns =====

type CreateCampaignInput struct {
	TeamID           string   `json:"team_id" jsonschema:"Team ID"`
	Name             string   `json:"name" jsonschema:"Campaign name"`
	Structure        string   `json:"structure" jsonschema:"Campaign structure as JSON string"`
	RequiredHashtags []string `json:"required_hashtags,omitempty" jsonschema:"Hashtags to include in posts"`
}

type CreateCampaignOutput struct {
	CampaignID string `json:"campaign_id"`
	Name       string `json:"name"`
}

type GetCampaignInput struct {
	TeamID     string `json:"team_id" jsonschema:"Team ID"`
	CampaignID string `json:"campaign_id" jsonschema:"Campaign format ID"`
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
	TeamID         string   `json:"team_id" jsonschema:"Team ID"`
	Title          string   `json:"title" jsonschema:"Post title with optional {{variables}}"`
	Content        string   `json:"content" jsonschema:"Post content with optional {{variables}}"`
	RecurrenceJSON string   `json:"recurrence_json" jsonschema:"RRULE string (e.g., FREQ=WEEKLY;BYDAY=TU)"`
	TargetAccounts []string `json:"target_accounts" jsonschema:"List of account IDs"`
	Visibility     string   `json:"visibility,omitempty" jsonschema:"public,unlisted,private,direct"`
	Enabled        *bool    `json:"enabled,omitempty" jsonschema:"Enable immediately (default: true)"`
}

type CreateRecurringOutput struct {
	TemplateID string `json:"template_id"`
}

// ===== RSS Feeds =====

type CreateRSSFeedInput struct {
	TeamID           string   `json:"team_id" jsonschema:"Team ID"`
	FeedURL          string   `json:"feed_url" jsonschema:"RSS feed URL"`
	Name             string   `json:"name" jsonschema:"Feed name"`
	ContentTemplate  string   `json:"content_template,omitempty" jsonschema:"Content template with {title}, {link}, {description}"`
	TargetAccountIDs []string `json:"target_account_ids" jsonschema:"List of account IDs"`
	OutputMode       string   `json:"output_mode,omitempty" jsonschema:"draft,scheduled,publish_now"`
}

type CreateRSSFeedOutput struct {
	FeedID string `json:"feed_id"`
}

// ===== Calendar =====

type GetCalendarInput struct {
	TeamID   string `json:"team_id" jsonschema:"Team ID"`
	FromDate string `json:"from_date,omitempty" jsonschema:"Start date (RFC3339, default: -7 days)"`
	ToDate   string `json:"to_date,omitempty" jsonschema:"End date (RFC3339, default: +7 days)"`
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
	TeamID     string `json:"team_id" jsonschema:"Team ID"`
	Weekday    string `json:"weekday,omitempty" jsonschema:"monday,tuesday,wednesday,thursday,friday,saturday,sunday"`
	AfterDate  string `json:"after_date,omitempty" jsonschema:"Start searching from (RFC3339, default: now)"`
	BeforeDate string `json:"before_date,omitempty" jsonschema:"Stop searching at (RFC3339, default: +30 days)"`
}

type FindFreeSlotOutput struct {
	Date      string `json:"date,omitempty"`
	Weekday   string `json:"weekday,omitempty"`
	Available bool   `json:"available"`
}

// ===== Schedule Post =====

type SchedulePostInput struct {
	TeamID                 string            `json:"team_id" jsonschema:"Team ID"`
	Title                  string            `json:"title" jsonschema:"Post title (required, set it explicitly)"`
	Content                string            `json:"content" jsonschema:"Post content"`
	ScheduledAt            string            `json:"scheduled_at" jsonschema:"Schedule datetime (RFC3339)"`
	TargetAccounts         []string          `json:"target_accounts" jsonschema:"List of account IDs"`
	Visibility             string            `json:"visibility,omitempty" jsonschema:"public,unlisted,private,direct"`
	AccountContentOverride map[string]string `json:"account_content_override,omitempty" jsonschema:"Per-account content for character limits"`
}

type SchedulePostOutput struct {
	PostID      string `json:"post_id"`
	ScheduledAt string `json:"scheduled_at"`
	Status      string `json:"status"`
}

// ===== Draft Post =====

type DraftPostInput struct {
	TeamID                 string            `json:"team_id" jsonschema:"Team ID"`
	Title                  string            `json:"title" jsonschema:"Post title (required, set it explicitly)"`
	Content                string            `json:"content" jsonschema:"Post content"`
	TargetAccounts         []string          `json:"target_accounts" jsonschema:"List of account IDs"`
	Visibility             string            `json:"visibility,omitempty" jsonschema:"public,unlisted,private,direct"`
	AccountContentOverride map[string]string `json:"account_content_override,omitempty" jsonschema:"Per-account content for character limits"`
}

type DraftPostOutput struct {
	PostID string `json:"post_id"`
	Status string `json:"status"`
}

// ===== Get Posts =====

type GetPostsInput struct {
	TeamID string `json:"team_id" jsonschema:"Team ID"`
	Status string `json:"status,omitempty" jsonschema:"pending,processing,posted,failed,cancelled,draft"`
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
	TeamID                 string            `json:"team_id" jsonschema:"Team ID"`
	PostID                 string            `json:"post_id" jsonschema:"Post ID to modify"`
	Title                  *string           `json:"title,omitempty" jsonschema:"New title (null to keep current)"`
	Content                *string           `json:"content,omitempty" jsonschema:"New content (null to keep current)"`
	ScheduledAt            *string           `json:"scheduled_at,omitempty" jsonschema:"New schedule (RFC3339, null to keep)"`
	TargetAccounts         *[]string         `json:"target_accounts,omitempty" jsonschema:"New target accounts (null to keep)"`
	Visibility             *string           `json:"visibility,omitempty" jsonschema:"New visibility (null to keep)"`
	AccountContentOverride map[string]string `json:"account_content_override,omitempty" jsonschema:"Per-account content overrides"`
}

type ModifyPostOutput struct {
	PostID string `json:"post_id"`
	Status string `json:"status"`
}

// ===== Delete Post =====

type DeletePostInput struct {
	TeamID string `json:"team_id" jsonschema:"Team ID"`
	PostID string `json:"post_id" jsonschema:"Post ID to delete"`
}

type DeletePostOutput struct {
	Success bool `json:"success"`
}

// ===== Get Platforms =====

type GetPlatformsInput struct {
	TeamID string `json:"team_id" jsonschema:"Team ID"`
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
	TeamID string `json:"team_id" jsonschema:"Team ID"`
}

type BrandProfileOutput struct {
	HasProfile      bool     `json:"has_profile"`
	Tonality        string   `json:"tonality,omitempty"`
	FormattingRules []string `json:"formatting_rules,omitempty"`
	BannedWords     []string `json:"banned_words,omitempty"`
	MaxHashtags     int      `json:"max_hashtags,omitempty"`
	Industry        string   `json:"industry,omitempty"`
	TargetAudience  string   `json:"target_audience,omitempty"`
}

// ===== Search Posts =====

type SearchPostsInput struct {
	TeamID   string `json:"team_id" jsonschema:"Team ID"`
	Query    string `json:"query" jsonschema:"Search query (matches content)"`
	FromDate string `json:"from_date,omitempty" jsonschema:"Start date (RFC3339)"`
	ToDate   string `json:"to_date,omitempty" jsonschema:"End date (RFC3339)"`
	Status   string `json:"status,omitempty" jsonschema:"Filter by status"`
}

type SearchPostsOutput struct {
	Posts []PostSummary `json:"posts"`
	Total int           `json:"total"`
}

// ===== Get Analytics =====

type GetAnalyticsInput struct {
	TeamID string `json:"team_id" jsonschema:"Team ID"`
}

type MetricValue struct {
	Metric string `json:"metric"`
	Total  int64  `json:"total"`
}

type GetAnalyticsOutput struct {
	Metrics  []MetricValue `json:"metrics"`
	TopPosts []PostSummary `json:"top_posts,omitempty"`
}

// ===== Get Hashtag Performance =====

type GetHashtagPerformanceInput struct {
	TeamID   string `json:"team_id" jsonschema:"Team ID"`
	Days     int    `json:"days,omitempty" jsonschema:"Time window in days (default 90; max 366)"`
	Provider string `json:"provider,omitempty" jsonschema:"Filter by platform: bluesky; mastodon or friendica. Omit for all platforms"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum number of hashtags (default 20; max 50)"`
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

// ===== Get Analytics Timeslots =====

type GetAnalyticsTimeslotsInput struct {
	TeamID   string `json:"team_id" jsonschema:"Team ID"`
	Days     int    `json:"days,omitempty" jsonschema:"Time window in days (default 90; max 366)"`
	Provider string `json:"provider,omitempty" jsonschema:"Filter by platform: bluesky; mastodon or friendica. Omit for all platforms"`
	Timezone string `json:"timezone,omitempty" jsonschema:"IANA timezone for weekday/hour buckets (e.g. Europe/Berlin). Default UTC"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum number of timeslots to return (default 5; max 50)"`
}

type TimeslotValue struct {
	Weekday         string  `json:"weekday"`
	Hour            int     `json:"hour"`
	Posts           int     `json:"posts"`
	TotalEngagement int64   `json:"total_engagement"`
	AvgEngagement   float64 `json:"avg_engagement"`
	Score           float64 `json:"score"`
}

type GetAnalyticsTimeslotsOutput struct {
	Timezone  string          `json:"timezone"`
	Timeslots []TimeslotValue `json:"timeslots"`
}

// ===== Get Current View (chat only) =====

type GetCurrentViewInput struct{}
