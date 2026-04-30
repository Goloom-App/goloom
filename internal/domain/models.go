package domain

import (
	"errors"
	"time"
)

type TeamRole string

const (
	RoleOwner  TeamRole = "owner"
	RoleEditor TeamRole = "editor"
	RoleViewer TeamRole = "viewer"
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
	CreatedAt time.Time `json:"created_at"`
}

type Team struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type TeamMembership struct {
	UserID    string    `json:"user_id"`
	TeamID    string    `json:"team_id"`
	Role      TeamRole  `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type SocialAccount struct {
	ID                     string    `json:"id"`
	TeamID                 string    `json:"team_id"`
	Provider               string    `json:"provider"`
	InstanceURL            string    `json:"instance_url"`
	Username               string    `json:"username"`
	RemoteAccountID        string    `json:"remote_account_id"`
	AccessTokenCiphertext  string    `json:"-"`
	RefreshTokenCiphertext string    `json:"-"`
	MaxCharsOverride       *int      `json:"max_chars_override,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
}

type ScheduledPost struct {
	ID             string     `json:"id"`
	TeamID         string     `json:"team_id"`
	AuthorUserID   string     `json:"author_user_id"`
	Content        string     `json:"content"`
	ScheduledAt    time.Time  `json:"scheduled_at"`
	Status         PostStatus `json:"status"`
	AttemptCount   int        `json:"attempt_count"`
	LastError      string     `json:"last_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	TargetAccounts []string   `json:"target_accounts"`
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
	User User
	Kind string
}

type CreateAccountInput struct {
	Provider        string `json:"provider"`
	InstanceURL     string `json:"instance_url"`
	Username        string `json:"username"`
	RemoteAccountID string `json:"remote_account_id"`
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
}

type CreatePostInput struct {
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
