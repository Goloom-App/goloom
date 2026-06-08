package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrProviderInstanceInUse is returned when deleting a provider instance that still has social accounts linked.
var ErrProviderInstanceInUse = errors.New("provider instance still has connected social accounts")

// ErrProviderInstanceNotFound is returned when deleting a provider instance that does not exist.
var ErrProviderInstanceNotFound = errors.New("provider instance not found")

// BootstrapAdminSubject is the fixed users.subject for the bootstrap / API-token administrator.
const BootstrapAdminSubject = "local-admin"

type TeamRole string

const (
	RoleOwner  TeamRole = "owner"
	RoleEditor TeamRole = "editor"
	RoleViewer TeamRole = "viewer"
)

type AccountAuthType string

const (
	AccountAuthTypeOAuthToken  AccountAuthType = "oauth_token"
	AccountAuthTypeAppPassword AccountAuthType = "app_password"
)

type PostStatus string

const (
	PostStatusPending    PostStatus = "pending"
	PostStatusProcessing PostStatus = "processing"
	PostStatusPosted     PostStatus = "posted"
	PostStatusFailed     PostStatus = "failed"
	PostStatusCancelled  PostStatus = "cancelled"
	PostStatusDraft      PostStatus = "draft"
)

type PostSource string

const (
	PostSourceScheduled  PostSource = "scheduled"
	PostSourceImported   PostSource = "imported"
	PostSourceAutomation PostSource = "automation"
)

type AIJobType string

const (
	AIJobTypeVoiceEngine       AIJobType = "voice_engine"
	AIJobTypeCampaignAutopilot AIJobType = "campaign_autopilot"
	AIJobTypeProactiveTrigger  AIJobType = "proactive_trigger"
	AIJobTypeProfileAnalysis   AIJobType = "profile_analysis"
)

type AIJobStatus string

const (
	AIJobStatusPending    AIJobStatus = "pending"
	AIJobStatusProcessing AIJobStatus = "processing"
	AIJobStatusCompleted  AIJobStatus = "completed"
	AIJobStatusFailed     AIJobStatus = "failed"
)

// Post visibility values aligned with Mastodon API.
const (
	PostVisibilityPublic   = "public"
	PostVisibilityUnlisted = "unlisted"
	PostVisibilityPrivate  = "private"
	PostVisibilityDirect   = "direct"
)

// NormalizePostVisibility returns a supported visibility or public.
func NormalizePostVisibility(v string) string {
	n := strings.ToLower(strings.TrimSpace(v))
	switch n {
	case PostVisibilityPublic, PostVisibilityUnlisted, PostVisibilityPrivate, PostVisibilityDirect:
		return n
	default:
		return PostVisibilityPublic
	}
}

// NormalizeAccountContentOverride keeps only overrides for accounts targeted by this post.
func NormalizeAccountContentOverride(over map[string]string, targetAccounts []string) map[string]string {
	if len(over) == 0 {
		return nil
	}
	targets := make(map[string]struct{})
	for _, id := range targetAccounts {
		targets[id] = struct{}{}
	}
	out := make(map[string]string)
	for accountID, content := range over {
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		if _, ok := targets[accountID]; !ok {
			continue
		}
		out[accountID] = content
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NormalizeMediaIDs trims and drops empty platform media IDs.
func NormalizeMediaIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// NormalizeMediaExcludeByAccount trims keys/values and keeps only exclusions for attachments on this post.
func NormalizeMediaExcludeByAccount(ex map[string][]string, mediaIDs []string) map[string][]string {
	if len(ex) == 0 {
		return nil
	}
	allowed := make(map[string]struct{})
	for _, id := range NormalizeMediaIDs(mediaIDs) {
		allowed[id] = struct{}{}
	}
	out := make(map[string][]string)
	for accountID, xs := range ex {
		accountID = strings.TrimSpace(accountID)
		if accountID == "" {
			continue
		}
		var filtered []string
		for _, mid := range xs {
			mid = strings.TrimSpace(mid)
			if mid == "" {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[mid]; !ok {
					continue
				}
			}
			filtered = append(filtered, mid)
		}
		filtered = NormalizeMediaIDs(filtered)
		if len(filtered) > 0 {
			out[accountID] = filtered
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FilterMediaIDsForAccount returns attachments this destination publishes after exclusions.
func FilterMediaIDsForAccount(all []string, excludeByAccount map[string][]string, accountID string) []string {
	all = NormalizeMediaIDs(all)
	if len(all) == 0 || excludeByAccount == nil {
		return append([]string(nil), all...)
	}
	xs := excludeByAccount[strings.TrimSpace(accountID)]
	if len(xs) == 0 {
		return append([]string(nil), all...)
	}
	drop := make(map[string]struct{})
	for _, id := range xs {
		id = strings.TrimSpace(id)
		if id != "" {
			drop[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(all))
	for _, id := range all {
		if _, omit := drop[id]; !omit {
			out = append(out, id)
		}
	}
	return out
}

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

type Team struct {
	ID                string                    `json:"id"`
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	IsPersonal        bool                      `json:"is_personal"`
	IsAIEnabled       bool                      `json:"is_ai_enabled"`
	PersonalForUserID string                    `json:"personal_for_user_id,omitempty"`
	SchedulingPrefs   TeamSchedulingPreferences `json:"scheduling_preferences"`
	CreatedAt         time.Time                 `json:"created_at"`
}

type StyleMetadata struct {
	Tonality          string   `json:"tonality"`
	FormattingRules   []string `json:"formatting_rules"`
	BannedWords       []string `json:"banned_words"`
	MaxHashtags       int      `json:"max_hashtags"`
	PreferredLanguage string   `json:"preferred_language"`
}

type TeamProfile struct {
	ID                 string        `json:"id"`
	TeamID             string        `json:"team_id"`
	StyleMetadata      StyleMetadata `json:"style_metadata"`
	AutoPublishEnabled bool          `json:"auto_publish_enabled"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

type CampaignFormat struct {
	ID               string          `json:"id"`
	TeamID           string          `json:"team_id"`
	Name             string          `json:"name"`
	Weekday          *int            `json:"weekday,omitempty"`
	Structure        json.RawMessage `json:"structure"`
	RequiredHashtags []string        `json:"required_hashtags"`
	IsActive         bool            `json:"is_active"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type StyleExample struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"team_id"`
	Platform  string    `json:"platform"`
	Content   string    `json:"content"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

type AIJob struct {
	ID           string          `json:"id"`
	TeamID       string          `json:"team_id"`
	AuthorUserID string          `json:"author_user_id"`
	Type         AIJobType       `json:"type"`
	Status       AIJobStatus     `json:"status"`
	Payload      json.RawMessage `json:"payload"`
	Result       json.RawMessage `json:"result"`
	ErrorMessage string          `json:"error_message,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

type AIServiceConfig struct {
	ID          string    `json:"id"`
	TeamID      *string   `json:"team_id,omitempty"`
	ServiceURL  string    `json:"service_url"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type RSSInitialSyncMode string

const (
	RSSInitialSyncBaseline      RSSInitialSyncMode = "baseline"
	RSSInitialSyncPublishLatest RSSInitialSyncMode = "publish_latest"
)

func NormalizeRSSInitialSyncMode(raw string) RSSInitialSyncMode {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case string(RSSInitialSyncPublishLatest):
		return RSSInitialSyncPublishLatest
	default:
		return RSSInitialSyncBaseline
	}
}

type AutomationOutputMode string

const (
	AutomationOutputDraft      AutomationOutputMode = "draft"
	AutomationOutputScheduled  AutomationOutputMode = "scheduled"
	AutomationOutputPublishNow AutomationOutputMode = "publish_now"
)

const DefaultRSSContentTemplate = "{title}\n\n{link}"

func NormalizeAutomationOutputMode(raw string) AutomationOutputMode {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case string(AutomationOutputScheduled):
		return AutomationOutputScheduled
	case string(AutomationOutputPublishNow):
		return AutomationOutputPublishNow
	default:
		return AutomationOutputDraft
	}
}

type RSSFeedConfig struct {
	ID               string               `json:"id"`
	TeamID           string               `json:"team_id"`
	FeedURL          string               `json:"feed_url"`
	Name             string               `json:"name"`
	IsActive         bool                 `json:"is_active"`
	AiEnhanceEnabled bool                 `json:"ai_enhance_enabled"`
	ContentTemplate  string               `json:"content_template"`
	OutputMode       AutomationOutputMode `json:"output_mode"`
	MaxPostsPerDay   int                  `json:"max_posts_per_day"`
	CounterNext      int                  `json:"counter_next"`
	PromptHint       string               `json:"prompt_hint"`
	TargetAccountIDs []string             `json:"target_account_ids"`
	Tonality         string               `json:"tonality"`
	InitialSyncMode  RSSInitialSyncMode   `json:"initial_sync_mode"`
	LastFetchedAt    *time.Time           `json:"last_fetched_at,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
}

func (f RSSFeedConfig) NormalizedContentTemplate() string {
	if strings.TrimSpace(f.ContentTemplate) == "" {
		return DefaultRSSContentTemplate
	}
	return f.ContentTemplate
}

func (f RSSFeedConfig) NormalizedMaxPostsPerDay() int {
	if f.MaxPostsPerDay <= 0 {
		return 10
	}
	return f.MaxPostsPerDay
}

type ProactiveTriggerSettings struct {
	ID                      string    `json:"id"`
	TeamID                  string    `json:"team_id"`
	ContentGapThresholdDays int       `json:"content_gap_threshold_days"`
	AutoFillEnabled         bool      `json:"auto_fill_enabled"`
	MaxTriggersPerDay       int       `json:"max_triggers_per_day"`
	CronSchedule            string    `json:"cron_schedule"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// ExternalPostMonitorSettings controls automatic import of posts published outside goloom.
type ExternalPostMonitorSettings struct {
	ID                  string     `json:"id,omitempty"`
	TeamID              string     `json:"team_id"`
	Enabled             bool       `json:"enabled"`
	BackfillCompletedAt *time.Time `json:"backfill_completed_at,omitempty"`
	LastSyncAt          *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at,omitempty"`
	UpdatedAt           time.Time  `json:"updated_at,omitempty"`
}

// UpsertExternalPostMonitorInput is the PUT body for external post monitor settings.
type UpsertExternalPostMonitorInput struct {
	Enabled bool `json:"enabled"`
}

// ImportedPostInput carries provider post data for CreateImportedPost.
type ImportedPostInput struct {
	AccountID       string
	RemotePostID    string
	Content         string
	PublishedAt     time.Time
	PublishedURL    string
	PublishMetadata map[string]string
}

type AIAccountSummary struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Username string `json:"username"`
	MaxChars int    `json:"max_chars"`
}

type AIContext struct {
	Team            Team                   `json:"team"`
	Profile         *TeamProfile           `json:"profile,omitempty"`
	CampaignFormats []CampaignFormat       `json:"campaign_formats"`
	StyleExamples   []StyleExample         `json:"style_examples"`
	RecentPosts     []ScheduledPost        `json:"recent_posts"`
	Accounts        []AIAccountSummary     `json:"accounts,omitempty"`
	UpcomingPosts   []ScheduledPost        `json:"upcoming_posts,omitempty"`
	EngagementHours []EngagementHourBucket `json:"engagement_hours,omitempty"`
}

const AIContextRecentPostsLimit = 100
const AIContextUpcomingPostsLimit = 200

// MaxCharsForProvider returns the default character limit for a provider, honoring per-account overrides.
func MaxCharsForProvider(provider string, override *int) int {
	if override != nil && *override > 0 {
		return *override
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "bluesky":
		return 300
	case "friendica":
		return 5000
	case "mastodon":
		return 500
	default:
		return 500
	}
}

type TeamInvitation struct {
	ID              string    `json:"id"`
	TeamID          string    `json:"team_id"`
	Email           string    `json:"email"`
	Role            TeamRole  `json:"role"`
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedByUserID string    `json:"created_by_user_id"`
	CreatedAt       time.Time `json:"created_at"`
}

type CreateTeamInvitationInput struct {
	Email string   `json:"email"`
	Role  TeamRole `json:"role"`
}

type AcceptTeamInvitationInput struct {
	Token string `json:"token"`
}

type MigrateAccountInput struct {
	TargetTeamID string `json:"target_team_id"`
}

type TeamMembership struct {
	UserID    string    `json:"user_id"`
	TeamID    string    `json:"team_id"`
	Role      TeamRole  `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type SocialAccount struct {
	ID                     string          `json:"id"`
	TeamID                 string          `json:"team_id"`
	Name                   string          `json:"name"`
	Provider               string          `json:"provider"`
	AuthType               AccountAuthType `json:"auth_type"`
	ProviderInstanceID     string          `json:"provider_instance_id,omitempty"`
	InstanceURL            string          `json:"instance_url"`
	Username               string          `json:"username"`
	RemoteAccountID        string          `json:"remote_account_id"`
	AvatarURL              string          `json:"avatar_url,omitempty"`
	AccessTokenCiphertext  string          `json:"-"`
	RefreshTokenCiphertext string          `json:"-"`
	MaxCharsOverride       *int            `json:"max_chars_override,omitempty"`
	AccessTokenExpiresAt   *time.Time      `json:"access_token_expires_at,omitempty"`
	AccountMetricsSyncedAt *time.Time      `json:"account_metrics_synced_at,omitempty"`
	PostEngagementSyncedAt *time.Time      `json:"post_engagement_synced_at,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
}

type ScheduledPost struct {
	ID                    string              `json:"id"`
	TeamID                string              `json:"team_id"`
	AuthorUserID          string              `json:"author_user_id"`
	Title                 string              `json:"title"`
	Content               string              `json:"content"`
	ScheduledAt           time.Time           `json:"scheduled_at"`
	Status                PostStatus          `json:"status"`
	Source                PostSource          `json:"source"`
	AttemptCount          int                 `json:"attempt_count"`
	LastError             string              `json:"last_error,omitempty"`
	CreatedAt             time.Time           `json:"created_at"`
	UpdatedAt             time.Time           `json:"updated_at"`
	TargetAccounts        []string            `json:"target_accounts"`
	PublishedLinks        map[string]string   `json:"published_links,omitempty"`
	Visibility            string              `json:"visibility,omitempty"`
	MediaIDs              []string            `json:"media_ids,omitempty"`
	MediaExcludeByAccount map[string][]string `json:"media_exclude_by_account,omitempty"`
	PostTemplateID        *string             `json:"post_template_id,omitempty"`
	TemplateCounter       *int                `json:"template_counter,omitempty"`
	RSSFeedID             *string             `json:"rss_feed_id,omitempty"`
}

// PostTemplate drives recurring scheduled posts (stored in post_templates).
type PostTemplate struct {
	ID                     string              `json:"id"`
	TeamID                 string              `json:"team_id"`
	AuthorUserID           string              `json:"author_user_id"`
	Title                  string              `json:"title"`
	Content                string              `json:"content"`
	RecurrenceJSON         string              `json:"recurrence_json"`
	Visibility             string              `json:"visibility"`
	MediaIDs               []string            `json:"media_ids,omitempty"`
	MediaExcludeByAccount  map[string][]string `json:"media_exclude_by_account,omitempty"`
	TargetAccountIDs       []string            `json:"target_account_ids"`
	Enabled                bool                `json:"enabled"`
	AiEnhanceEnabled       bool                `json:"ai_enhance_enabled"`
	OutputMode             AutomationOutputMode `json:"output_mode"`
	PromptHint             string              `json:"prompt_hint"`
	Tonality               string              `json:"tonality"`
	NextMaterializeAt      *time.Time          `json:"next_materialize_at,omitempty"`
	CounterNext            int                 `json:"counter_next"`
	AnnouncesTemplateID    *string             `json:"announces_template_id,omitempty"`
	AnnouncementDaysBefore *int                `json:"announcement_days_before,omitempty"`
	CreatedAt              time.Time           `json:"created_at"`
	UpdatedAt              time.Time           `json:"updated_at"`
}

type CreatePostTemplateInput struct {
	Title                  string              `json:"title"`
	Content                string              `json:"content"`
	RecurrenceJSON         string              `json:"recurrence_json"`
	Visibility             string              `json:"visibility,omitempty"`
	MediaIDs               []string            `json:"media_ids,omitempty"`
	MediaExcludeByAccount  map[string][]string `json:"media_exclude_by_account,omitempty"`
	TargetAccountIDs       []string            `json:"target_account_ids"`
	Enabled                *bool               `json:"enabled,omitempty"`
	AiEnhanceEnabled       *bool               `json:"ai_enhance_enabled,omitempty"`
	OutputMode             AutomationOutputMode `json:"output_mode,omitempty"`
	PromptHint             string              `json:"prompt_hint,omitempty"`
	Tonality               string              `json:"tonality,omitempty"`
	AnnouncesTemplateID    *string             `json:"announces_template_id,omitempty"`
	AnnouncementDaysBefore *int                `json:"announcement_days_before,omitempty"`
}

type UpdatePostTemplateInput struct {
	Title                  *string              `json:"title,omitempty"`
	Content                *string              `json:"content,omitempty"`
	RecurrenceJSON         *string              `json:"recurrence_json,omitempty"`
	Visibility             *string              `json:"visibility,omitempty"`
	MediaIDs               *[]string            `json:"media_ids,omitempty"`
	MediaExcludeByAccount  map[string][]string  `json:"media_exclude_by_account,omitempty"`
	TargetAccountIDs       *[]string            `json:"target_account_ids,omitempty"`
	Enabled                *bool                `json:"enabled,omitempty"`
	AiEnhanceEnabled       *bool                `json:"ai_enhance_enabled,omitempty"`
	OutputMode             *AutomationOutputMode `json:"output_mode,omitempty"`
	PromptHint             *string              `json:"prompt_hint,omitempty"`
	Tonality               *string              `json:"tonality,omitempty"`
	AnnouncesTemplateID    *string              `json:"announces_template_id,omitempty"`
	AnnouncementDaysBefore *int                 `json:"announcement_days_before,omitempty"`
}

// EngagementHourBucket aggregates engagement score by UTC hour-of-day for posted content.
type EngagementHourBucket struct {
	HourUTC int   `json:"hour"`
	Score   int64 `json:"score"`
}

type ScheduledPostTarget struct {
	PostID       string     `json:"post_id"`
	AccountID    string     `json:"account_id"`
	Status       PostStatus `json:"status"`
	LastError    string     `json:"last_error,omitempty"`
	PublishedURL string     `json:"published_url,omitempty"`
}

// PostMetric stores aggregated engagement for one post, account, and metric name.
type PostMetric struct {
	PostID    string    `json:"post_id"`
	AccountID string    `json:"account_id"`
	Metric    string    `json:"metric"`
	Value     int64     `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PostVersion holds per-account content override for a scheduled post.
type PostVersion struct {
	PostID    string `json:"post_id"`
	AccountID string `json:"account_id"`
	Content   string `json:"content"`
}

// PostedTargetForMetricSync is a posted target row with the linked social account for metric polling.
type PostedTargetForMetricSync struct {
	PostID          string
	PublishedURL    string
	PublishMetadata map[string]string
	Account         SocialAccount
}

// AdminSyncStatus summarizes metrics sync scheduler state for the admin dashboard.
type AdminSyncStatus struct {
	PostMetricsSyncInterval     string `json:"post_metrics_sync_interval"`
	AccountMetricsSyncInterval  string `json:"account_metrics_sync_interval"`
	AccountHealthInterval       string `json:"account_health_interval"`
	PostedTargetsPendingSync    int    `json:"posted_targets_pending_sync"`
	PostedTargetsNeverSynced    int    `json:"posted_targets_never_synced"`
	PostedTargetsWithMetrics    int    `json:"posted_targets_with_metrics"`
	AccountsWithFollowerMetrics int    `json:"accounts_with_follower_metrics"`
}

// PostEngagementSummary ranks a posted scheduled post by summed metrics.
type PostEngagementSummary struct {
	PostID string `json:"post_id"`
	Title  string `json:"title"`
	Score  int64  `json:"score"`
}

// TeamAnalyticsSummary aggregates stored metrics for posted content in a team.
type TeamAnalyticsSummary struct {
	MetricsTotal map[string]int64        `json:"metrics_total"`
	TopPosts     []PostEngagementSummary `json:"top_posts"`
}

// TeamMetricDelta combines live post_metrics totals with day-over-day change from post_metrics_history.
type TeamMetricDelta struct {
	Metric         string   `json:"metric"`
	Total          int64    `json:"total"`
	DeltaVsPrevDay int64    `json:"delta_vs_prev_day"`
	DeltaPercent   *float64 `json:"delta_percent,omitempty"`
}

// TeamAnalyticsReport is returned by GET /v1/teams/{teamID}/analytics/summary.
type TeamAnalyticsReport struct {
	Metrics  []TeamMetricDelta       `json:"metrics"`
	TopPosts []PostEngagementSummary `json:"top_posts"`
}

// PostAnalyticsListRow is one posted row for GET /v1/teams/{teamID}/analytics/posts.
type PostAnalyticsListRow struct {
	PostID      string    `json:"post_id"`
	Title       string    `json:"title"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Score       int64     `json:"score"`
}

type MediaItem struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"team_id"`
	Sha256    string    `json:"sha256"`
	Filename  string    `json:"filename"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	Width     *int      `json:"width,omitempty"`
	Height    *int      `json:"height,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type MediaProviderMapping struct {
	MediaID   string     `json:"media_id"`
	AccountID string     `json:"account_id"`
	RemoteID  string     `json:"remote_id"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// MetricHistoryPoint is one day in GET /v1/teams/{teamID}/analytics/chart time series.
type MetricHistoryPoint struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
}

type AccountMetricHistoryPoint struct {
	Date      string `json:"date"`
	Followers int64  `json:"followers"`
	Following int64  `json:"following"`
	Posts     int64  `json:"posts"`
}

// WebSessionAPITokenName is the reserved api_tokens.name for browser session bearer tokens.
const WebSessionAPITokenName = "__web_session"

// APITokenExpired reports whether a token is past its expires_at (tokens without expiry never expire).
func APITokenExpired(expiresAt *time.Time, now time.Time) bool {
	if expiresAt == nil {
		return false
	}
	return !expiresAt.After(now)
}

type APIToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"-"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type AuthenticatedPrincipal struct {
	User        User     `json:"user"`
	Kind        string   `json:"kind"`
	Scopes      []string `json:"scopes,omitempty"`
	TokenTeamID *string  `json:"token_team_id,omitempty"`
}

type CreateAccountInput struct {
	Provider           string `json:"provider"`
	ProviderInstanceID string `json:"provider_instance_id"`
	InstanceURL        string `json:"instance_url"`
	Username           string `json:"username"`
	Identifier         string `json:"identifier"`
	RemoteAccountID    string `json:"remote_account_id"`
	AccessToken        string `json:"access_token"`
	RefreshToken       string `json:"refresh_token"`
	AppPassword        string `json:"app_password"`
}

type UpdateAccountInput struct {
	Name             *string `json:"name,omitempty"`
	MaxCharsOverride *int    `json:"max_chars_override,omitempty"`
	AccessToken      *string `json:"access_token,omitempty"`
	RefreshToken     *string `json:"refresh_token,omitempty"`
}

type ConnectedAccount struct {
	Provider             string
	AuthType             AccountAuthType
	ProviderInstanceID   string
	InstanceURL          string
	Username             string
	RemoteAccountID      string
	AvatarURL            string
	AccessToken          string
	RefreshToken         string
	AccessTokenExpiresAt *time.Time
}

type ProviderInstance struct {
	ID                     string    `json:"id"`
	Provider               string    `json:"provider"`
	Name                   string    `json:"name"`
	InstanceURL            string    `json:"instance_url"`
	ClientID               string    `json:"client_id"`
	ClientSecretCiphertext string    `json:"-"`
	HasClientSecret        bool      `json:"has_client_secret"`
	Scopes                 []string  `json:"scopes"`
	AuthorizationEndpoint  string    `json:"authorization_endpoint,omitempty"`
	TokenEndpoint          string    `json:"token_endpoint,omitempty"`
	CreatedByUserID        string    `json:"created_by_user_id"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type CreateProviderInstanceInput struct {
	Provider              string   `json:"provider"`
	Name                  string   `json:"name"`
	InstanceURL           string   `json:"instance_url"`
	ClientID              string   `json:"client_id"`
	ClientSecret          string   `json:"client_secret"`
	Scopes                []string `json:"scopes"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
}

type PreparedProviderInstance struct {
	Provider              string
	Name                  string
	InstanceURL           string
	ClientID              string
	ClientSecret          string
	Scopes                []string
	AuthorizationEndpoint string
	TokenEndpoint         string
}

type CreateTeamInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UpdateTeamInput struct {
	Name                  string                     `json:"name"`
	Description           string                     `json:"description"`
	SchedulingPreferences *TeamSchedulingPreferences `json:"scheduling_preferences,omitempty"`
	IsAIEnabled           *bool                      `json:"is_ai_enabled,omitempty"`
}

// AdminMetrics aggregates global counts for the admin dashboard.
// AccountOAuthTokenExpiry carries token-expiry fields for background health checks.
type AccountOAuthTokenExpiry struct {
	ID                   string    `json:"id"`
	TeamID               string    `json:"team_id"`
	Provider             string    `json:"provider"`
	Username             string    `json:"username"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at"`
}

type AdminMetrics struct {
	UsersCount             int   `json:"users_count"`
	TeamsCount             int   `json:"teams_count"`
	ProviderInstancesCount int   `json:"provider_instances_count"`
	PostsPending           int64 `json:"posts_pending"`
	PostsDraft             int64 `json:"posts_draft"`
	PostsProcessing        int64 `json:"posts_processing"`
	PostsPosted            int64 `json:"posts_posted"`
	PostsFailed            int64 `json:"posts_failed"`
	PostsCancelled         int64 `json:"posts_cancelled"`
}

type AddTeamMemberInput struct {
	UserID string   `json:"user_id"`
	Role   TeamRole `json:"role"`
}

type UpdateUserInput struct {
	IsAdmin bool `json:"is_admin"`
}

type CreatePostInput struct {
	Title                 string              `json:"title"`
	Content               string              `json:"content"`
	ScheduledAt           time.Time           `json:"scheduled_at"`
	TargetAccounts        []string            `json:"target_accounts"`
	Visibility            string              `json:"visibility,omitempty"`
	MediaIDs              []string            `json:"media_ids,omitempty"`
	MediaExcludeByAccount map[string][]string `json:"media_exclude_by_account,omitempty"`
	Draft                 bool                `json:"draft,omitempty"`
	// AccountContentOverride allows per-account overrides for text validation and storage.
	AccountContentOverride map[string]string `json:"account_content_override,omitempty"`
	// UseVersions allows per-account content overrides to bypass global character limit validation.
	UseVersions bool `json:"use_versions,omitempty"`
	// Internal-only (workers): optional author override and template lineage for dynamic variables.
	AuthorUserID    *string    `json:"-"`
	PostTemplateID  *string    `json:"-"`
	TemplateCounter *int       `json:"-"`
	Source          PostSource `json:"-"`
	RSSFeedID       *string    `json:"-"`
}

func (in CreatePostInput) EffectiveContent(accountID string) string {
	if over, ok := in.AccountContentOverride[accountID]; ok && strings.TrimSpace(over) != "" {
		return over
	}
	return in.Content
}

func (in CreatePostInput) Validate() error {
	if in.Draft {
		return nil
	}
	if in.Content == "" {
		return errors.New("content is required")
	}
	if len(in.TargetAccounts) == 0 {
		return errors.New("target_accounts is required")
	}
	return nil
}

func (in TeamProfile) Validate() error {
	if strings.TrimSpace(in.TeamID) == "" {
		return errors.New("team_id is required")
	}
	if in.StyleMetadata.Tonality == "" && len(in.StyleMetadata.FormattingRules) == 0 && len(in.StyleMetadata.BannedWords) == 0 && in.StyleMetadata.MaxHashtags == 0 && in.StyleMetadata.PreferredLanguage == "" {
		return errors.New("style_metadata is required")
	}
	return nil
}

func (in CampaignFormat) Validate() error {
	if strings.TrimSpace(in.TeamID) == "" {
		return errors.New("team_id is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	if len(in.Structure) == 0 {
		return errors.New("structure is required")
	}
	return nil
}

func (in StyleExample) Validate() error {
	if strings.TrimSpace(in.TeamID) == "" {
		return errors.New("team_id is required")
	}
	if strings.TrimSpace(in.Platform) == "" {
		return errors.New("platform is required")
	}
	if strings.TrimSpace(in.Content) == "" {
		return errors.New("content is required")
	}
	return nil
}

// ResolvePostStatusOnUpdate returns the post row status after an update from CreatePostInput.
func ResolvePostStatusOnUpdate(was PostStatus, source PostSource, in CreatePostInput) PostStatus {
	if source == PostSourceImported {
		return PostStatusPosted
	}
	// If the post was already posted, processing or cancelled, do not change status back to pending/draft.
	if was == PostStatusPosted || was == PostStatusProcessing || was == PostStatusCancelled {
		return was
	}

	// Safety check: if the scheduled date is in the future,
	// and it's not explicitly a draft, ensure it's pending (or stays as it was if valid).
	// This prevents accidentally marking future posts as "posted" via API.
	if !in.Draft && in.ScheduledAt.After(time.Now()) {
		return PostStatusPending
	}

	if in.Draft {
		return PostStatusDraft
	}
	if was == PostStatusDraft {
		return PostStatusPending
	}
	return was
}

// LogEntry is a persisted log record captured from the structured logger.
type LogEntry struct {
	ID         string            `json:"id"`
	Level      string            `json:"level"`
	Message    string            `json:"message"`
	Attributes map[string]string `json:"attributes"`
	SourceFile string            `json:"source_file,omitempty"`
	SourceLine int               `json:"source_line,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	ArchivedAt *time.Time        `json:"archived_at,omitempty"`
}

// LogFilter specifies pagination and filtering for listing log entries.
type LogFilter struct {
	Level    string
	Search   string
	Archived *bool // nil = all, true = archived, false = unarchived
	Before   *time.Time
	After    *time.Time
	Limit    int
	Offset   int
}
