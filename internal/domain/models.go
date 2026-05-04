package domain

import (
	"errors"
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
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

type Team struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	IsPersonal       bool      `json:"is_personal"`
	PersonalForUserID string   `json:"personal_for_user_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
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
}

type ScheduledPostTarget struct {
	PostID       string     `json:"post_id"`
	AccountID    string     `json:"account_id"`
	Status       PostStatus `json:"status"`
	LastError    string     `json:"last_error,omitempty"`
	PublishedURL string     `json:"published_url,omitempty"`
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
	Provider           string
	AuthType           AccountAuthType
	ProviderInstanceID string
	InstanceURL        string
	Username           string
	RemoteAccountID    string
	AvatarURL          string
	AccessToken        string
	RefreshToken       string
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
type AdminMetrics struct {
	UsersCount               int   `json:"users_count"`
	TeamsCount               int   `json:"teams_count"`
	ProviderInstancesCount   int   `json:"provider_instances_count"`
	PostsPending             int64 `json:"posts_pending"`
	PostsProcessing          int64 `json:"posts_processing"`
	PostsPosted              int64 `json:"posts_posted"`
	PostsFailed              int64 `json:"posts_failed"`
	PostsCancelled           int64 `json:"posts_cancelled"`
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
}

func (in CreatePostInput) Validate() error {
	if in.Content == "" {
		return errors.New("content is required")
	}
	if len(in.TargetAccounts) == 0 {
		return errors.New("target_accounts is required")
	}
	return nil
}
