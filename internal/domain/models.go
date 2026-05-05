package domain

import (
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

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

type Team struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	IsPersonal        bool      `json:"is_personal"`
	PersonalForUserID string    `json:"personal_for_user_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
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
	CreatedAt              time.Time       `json:"created_at"`
}

type ScheduledPost struct {
	ID             string            `json:"id"`
	TeamID         string            `json:"team_id"`
	AuthorUserID   string            `json:"author_user_id"`
	Title          string            `json:"title"`
	Content        string            `json:"content"`
	ScheduledAt    time.Time         `json:"scheduled_at"`
	Status         PostStatus        `json:"status"`
	AttemptCount   int               `json:"attempt_count"`
	LastError      string            `json:"last_error,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	TargetAccounts []string          `json:"target_accounts"`
	PublishedLinks map[string]string `json:"published_links,omitempty"`
	Visibility     string            `json:"visibility,omitempty"`
	MediaIDs       []string          `json:"media_ids,omitempty"`
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
	PostID       string
	PublishedURL string
	Account      SocialAccount
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
	User User   `json:"user"`
	Kind string `json:"kind"`
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
	Title          string    `json:"title"`
	Content        string    `json:"content"`
	ScheduledAt    time.Time `json:"scheduled_at"`
	TargetAccounts []string  `json:"target_accounts"`
	Visibility     string    `json:"visibility,omitempty"`
	MediaIDs       []string  `json:"media_ids,omitempty"`
	Draft          bool      `json:"draft,omitempty"`
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

// ResolvePostStatusOnUpdate returns the post row status after an update from CreatePostInput.
func ResolvePostStatusOnUpdate(was PostStatus, in CreatePostInput) PostStatus {
	if in.Draft {
		switch was {
		case PostStatusPosted, PostStatusProcessing, PostStatusCancelled:
			return was
		default:
			return PostStatusDraft
		}
	}
	if was == PostStatusDraft {
		return PostStatusPending
	}
	return was
}
