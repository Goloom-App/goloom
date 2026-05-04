package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// fetchMastodonCompatibleAvatar loads an avatar URL from a Mastodon-compatible verify_credentials response.
func fetchMastodonCompatibleAvatar(ctx context.Context, instanceURL, accessToken string) string {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(instanceURL, "/")+"/api/v1/accounts/verify_credentials", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return ""
	}
	var payload struct {
		AvatarStatic string `json:"avatar_static"`
		Avatar       string `json:"avatar"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ""
	}
	if s := strings.TrimSpace(payload.AvatarStatic); s != "" {
		return s
	}
	return strings.TrimSpace(payload.Avatar)
}

func capabilitiesForAccount(account domain.SocialAccount, defaultChars int, mediaTypes []string) Capabilities {
	if account.MaxCharsOverride != nil {
		return Capabilities{MaxChars: *account.MaxCharsOverride, MediaTypes: mediaTypes}
	}
	return Capabilities{MaxChars: defaultChars, MediaTypes: mediaTypes}
}

// GenericStatusProvider posts to a Mastodon-compatible /api/v1/statuses endpoint (Friendica and similar).
type GenericStatusProvider struct {
	name         string
	defaultChars int
	mediaTypes   []string
	postPath     string
}

// NewFriendicaProvider returns a Friendica integration backed by the generic Mastodon API-compatible client.
func NewFriendicaProvider() SocialMediaProvider {
	return &GenericStatusProvider{
		name:         "friendica",
		defaultChars: 5000,
		mediaTypes:   []string{"image/jpeg", "image/png", "video/mp4"},
		postPath:     "/api/v1/statuses",
	}
}

func (p *GenericStatusProvider) Name() string {
	return p.name
}

func (p *GenericStatusProvider) Capabilities(_ context.Context, account domain.SocialAccount) (Capabilities, error) {
	return capabilitiesForAccount(account, p.defaultChars, p.mediaTypes), nil
}

func (p *GenericStatusProvider) PrepareProviderInstance(ctx context.Context, input domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error) {
	instanceURL, err := normalizeInstanceURL(ctx, input.InstanceURL)
	if err != nil {
		return domain.PreparedProviderInstance{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = strings.TrimPrefix(strings.TrimPrefix(instanceURL, "https://"), "http://")
	}
	return domain.PreparedProviderInstance{
		Provider:              p.name,
		Name:                  name,
		InstanceURL:           instanceURL,
		ClientID:              strings.TrimSpace(input.ClientID),
		ClientSecret:          strings.TrimSpace(input.ClientSecret),
		Scopes:                cleanScopes(input.Scopes),
		AuthorizationEndpoint: strings.TrimSpace(input.AuthorizationEndpoint),
		TokenEndpoint:         strings.TrimSpace(input.TokenEndpoint),
	}, nil
}

func (p *GenericStatusProvider) ConnectAccount(ctx context.Context, input domain.CreateAccountInput, instance *domain.ProviderInstance) (domain.ConnectedAccount, error) {
	instanceURL := strings.TrimSpace(input.InstanceURL)
	if instance != nil {
		instanceURL = instance.InstanceURL
	}
	normalizedURL, err := normalizeInstanceURL(ctx, instanceURL)
	if err != nil {
		return domain.ConnectedAccount{}, err
	}
	if strings.TrimSpace(input.Username) == "" || strings.TrimSpace(input.AccessToken) == "" {
		return domain.ConnectedAccount{}, errors.New("username and access_token are required")
	}
	acc := domain.ConnectedAccount{
		Provider:           p.name,
		AuthType:           domain.AccountAuthTypeOAuthToken,
		ProviderInstanceID: providerInstanceID(instance),
		InstanceURL:        normalizedURL,
		Username:           strings.TrimSpace(input.Username),
		RemoteAccountID:    strings.TrimSpace(input.RemoteAccountID),
		AccessToken:        strings.TrimSpace(input.AccessToken),
		RefreshToken:       strings.TrimSpace(input.RefreshToken),
	}
	acc.AvatarURL = fetchMastodonCompatibleAvatar(ctx, normalizedURL, acc.AccessToken)
	return acc, nil
}

func (p *GenericStatusProvider) UploadMedia(ctx context.Context, account domain.SocialAccount, auth PublishAuth, file io.Reader, filename, mimeType, altText string) (string, error) {
	return uploadMastodonV2Media(ctx, strings.TrimRight(account.InstanceURL, "/"), auth.AccessToken, file, filename, mimeType, altText)
}

func (p *GenericStatusProvider) Publish(ctx context.Context, account domain.SocialAccount, auth PublishAuth, req PublishRequest) (PublishResult, error) {
	payload := map[string]any{"status": req.Content}
	ids := domain.NormalizeMediaIDs(req.MediaIDs)
	if len(ids) > 0 {
		payload["media_ids"] = ids
	}
	vis := domain.NormalizePostVisibility(req.Visibility)
	if vis != "" {
		payload["visibility"] = vis
	}
	if req.ScheduledAt != nil && !req.ScheduledAt.IsZero() {
		payload["scheduled_at"] = req.ScheduledAt.UTC().Format(time.RFC3339Nano)
	}
	body, err := marshalJSONBody(payload)
	if err != nil {
		return PublishResult{}, err
	}

	resp, err := doJSONRequest(ctx, http.MethodPost, strings.TrimRight(account.InstanceURL, "/")+p.postPath, auth.AccessToken, body)
	if err != nil {
		return PublishResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return PublishResult{}, fmt.Errorf("%s publish failed with status %d", p.name, resp.StatusCode)
	}

	var statusPayload mastodonStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusPayload); err != nil {
		return PublishResult{}, fmt.Errorf("decode %s publish response: %w", p.name, err)
	}

	meta := map[string]string{}
	if strings.TrimSpace(statusPayload.URI) != "" {
		meta["uri"] = strings.TrimSpace(statusPayload.URI)
	}
	return PublishResult{RemoteID: statusPayload.ID, URL: statusPayload.URL, Metadata: meta}, nil
}

func (p *GenericStatusProvider) GetMetrics(ctx context.Context, account domain.SocialAccount, auth PublishAuth, publishedURL string) ([]EngagementMetric, error) {
	path := strings.TrimSpace(p.postPath)
	if path == "" {
		path = "/api/v1/statuses"
	}
	return fetchMastodonCompatibleMetrics(ctx, account.InstanceURL, auth.AccessToken, path, publishedURL)
}
