package provider

import (
	"context"
	"io"
	"sort"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// Capabilities describes what a connected account can do on a provider.
type Capabilities struct {
	MaxChars   int      `json:"max_chars"`
	MediaTypes []string `json:"media_types"`
}

// PublishRequest is the normalized payload for publishing a status update.
type PublishRequest struct {
	Content     string
	MediaIDs    []string
	Visibility  string
	ScheduledAt *time.Time
	// SpoilerText is Mastodon-style content warning (CW) text; empty means none.
	SpoilerText string
	// Sensitive marks media as sensitive where the provider supports it (e.g. Mastodon).
	Sensitive bool
}

// PublishAuth carries decrypted credentials for a publish call.
type PublishAuth struct {
	AccessToken  string
	RefreshToken string
}

// PublishResult is returned after a successful publish.
type PublishResult struct {
	RemoteID string
	URL      string
	Metadata map[string]string
}

// EngagementMetric is one normalized metric from a provider before persistence.
type EngagementMetric struct {
	Name  string
	Value int64
}

type AccountMetric = EngagementMetric

// OAuthAccountConnector is implemented by providers that support browser OAuth for account linking.
type OAuthAccountConnector interface {
	BuildAuthorizationURL(instance domain.ProviderInstance, state, redirectURI string) (string, error)
	ConnectAccountOAuthCallback(ctx context.Context, instance domain.ProviderInstance, clientSecret, redirectURI, code string) (domain.ConnectedAccount, error)
}

// OAuthTokenRefresher exchanges a refresh token for new credentials (Mastodon-compatible OAuth).
type OAuthTokenRefresher interface {
	RefreshAccessToken(ctx context.Context, instance domain.ProviderInstance, clientSecret, refreshToken string) (accessToken, newRefreshToken string, expiresAt *time.Time, err error)
}

// SocialMediaProvider is implemented by each supported network integration.
type SocialMediaProvider interface {
	Name() string
	Capabilities(ctx context.Context, account domain.SocialAccount) (Capabilities, error)
	PrepareProviderInstance(ctx context.Context, input domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error)
	ConnectAccount(ctx context.Context, input domain.CreateAccountInput, instance *domain.ProviderInstance) (domain.ConnectedAccount, error)
	UploadMedia(ctx context.Context, account domain.SocialAccount, auth PublishAuth, file io.Reader, filename, mimeType, altText string) (string, error)
	Publish(ctx context.Context, account domain.SocialAccount, auth PublishAuth, req PublishRequest) (PublishResult, error)
	GetMetrics(ctx context.Context, account domain.SocialAccount, auth PublishAuth, publishedURL string) ([]EngagementMetric, error)
	GetAccountMetrics(ctx context.Context, account domain.SocialAccount, auth PublishAuth) ([]AccountMetric, error)
}

// Registry resolves a provider implementation by canonical name.
type Registry struct {
	providers map[string]SocialMediaProvider
}

// NewRegistry builds a registry from the given provider implementations.
func NewRegistry(providers ...SocialMediaProvider) *Registry {
	items := make(map[string]SocialMediaProvider, len(providers))
	for _, item := range providers {
		items[item.Name()] = item
	}
	return &Registry{providers: items}
}

// Get returns a provider by name (case-insensitive).
func (r *Registry) Get(name string) (SocialMediaProvider, bool) {
	provider, ok := r.providers[strings.ToLower(name)]
	return provider, ok
}

// Supported lists registered provider names in stable sorted order.
func (r *Registry) Supported() []string {
	out := make([]string, 0, len(r.providers))
	for name := range r.providers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
