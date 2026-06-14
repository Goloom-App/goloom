package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

type mastodonAccountResponse struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Acct           string `json:"acct"`
	URL            string `json:"url"`
	AvatarStatic   string `json:"avatar_static"`
	Avatar         string `json:"avatar"`
	FollowersCount int64  `json:"followers_count"`
	FollowingCount int64  `json:"following_count"`
	StatusesCount  int64  `json:"statuses_count"`
}

type mastodonStatusResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
	URI string `json:"uri"`
}

type mastodonAuthorizationServerMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

type mastodonTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type mastodonAppRegistrationRequest struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
	Scopes       string   `json:"scopes,omitempty"`
	Website      string   `json:"website,omitempty"`
}

type mastodonAppRegistrationResponse struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
}

// MastodonRegistrationConfig controls automatic Mastodon app registration defaults.
type MastodonRegistrationConfig struct {
	AppName       string
	RedirectURI   string
	Website       string
	DefaultScopes []string
}

// MastodonProvider implements SocialMediaProvider for Mastodon and Mastodon-compatible
// OAuth networks (e.g. Pixelfed) that share the /api/v1 status & OAuth app endpoints.
type MastodonProvider struct {
	name          string
	defaultChars  int
	mediaTypes    []string
	requiresMedia bool
	registration  MastodonRegistrationConfig
}

func normalizeMastodonRegistration(cfg MastodonRegistrationConfig) MastodonRegistrationConfig {
	if strings.TrimSpace(cfg.AppName) == "" {
		cfg.AppName = "goloom"
	}
	if strings.TrimSpace(cfg.RedirectURI) == "" {
		cfg.RedirectURI = "urn:ietf:wg:oauth:2.0:oob"
	}
	if len(cfg.DefaultScopes) == 0 {
		cfg.DefaultScopes = []string{"read", "write"}
	}
	return cfg
}

// NewMastodonProvider returns a Mastodon integration with optional app registration defaults.
func NewMastodonProvider(cfg MastodonRegistrationConfig) SocialMediaProvider {
	return &MastodonProvider{
		name:         "mastodon",
		defaultChars: 500,
		mediaTypes:   []string{"image/jpeg", "image/png", "video/mp4"},
		registration: normalizeMastodonRegistration(cfg),
	}
}

// NewPixelfedProvider returns a Pixelfed integration. Pixelfed speaks the Mastodon API
// (OAuth app registration, /api/v1/statuses, media upload), but rejects text-only posts,
// so RequiresMedia is set and the default caption limit is higher.
func NewPixelfedProvider(cfg MastodonRegistrationConfig) SocialMediaProvider {
	return &MastodonProvider{
		name:          "pixelfed",
		defaultChars:  2000,
		mediaTypes:    []string{"image/jpeg", "image/png", "image/gif", "video/mp4"},
		requiresMedia: true,
		registration:  normalizeMastodonRegistration(cfg),
	}
}

func (p *MastodonProvider) Name() string {
	return p.name
}

func (p *MastodonProvider) Capabilities(_ context.Context, account domain.SocialAccount) (Capabilities, error) {
	caps := capabilitiesForAccount(account, p.defaultChars, p.mediaTypes)
	caps.RequiresMedia = p.requiresMedia
	return caps, nil
}

func (p *MastodonProvider) PrepareProviderInstance(ctx context.Context, input domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error) {
	instanceURL, err := normalizeInstanceURL(ctx, input.InstanceURL)
	if err != nil {
		return domain.PreparedProviderInstance{}, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = strings.TrimPrefix(strings.TrimPrefix(instanceURL, "https://"), "http://")
	}

	scopes := cleanScopes(input.Scopes)
	if len(scopes) == 0 {
		scopes = append([]string(nil), p.registration.DefaultScopes...)
	}
	if len(scopes) == 0 {
		scopes = []string{"read", "write"}
	}

	clientID := strings.TrimSpace(input.ClientID)
	clientSecret := strings.TrimSpace(input.ClientSecret)
	if clientID == "" || clientSecret == "" {
		registration, err := p.registerApplication(ctx, instanceURL, scopes)
		if err != nil {
			return domain.PreparedProviderInstance{}, err
		}
		clientID = registration.ClientID
		clientSecret = registration.ClientSecret
	}

	authEndpoint := strings.TrimSpace(input.AuthorizationEndpoint)
	tokenEndpoint := strings.TrimSpace(input.TokenEndpoint)
	if authEndpoint == "" || tokenEndpoint == "" {
		discoveredAuthEndpoint, discoveredTokenEndpoint, err := discoverMastodonOAuthMetadata(ctx, instanceURL)
		if err != nil {
			return domain.PreparedProviderInstance{}, err
		}
		if authEndpoint == "" {
			authEndpoint = discoveredAuthEndpoint
		}
		if tokenEndpoint == "" {
			tokenEndpoint = discoveredTokenEndpoint
		}
	}

	return domain.PreparedProviderInstance{
		Provider:              p.Name(),
		Name:                  name,
		InstanceURL:           instanceURL,
		ClientID:              clientID,
		ClientSecret:          clientSecret,
		Scopes:                scopes,
		AuthorizationEndpoint: authEndpoint,
		TokenEndpoint:         tokenEndpoint,
	}, nil
}

func (p *MastodonProvider) ConnectAccount(ctx context.Context, input domain.CreateAccountInput, instance *domain.ProviderInstance) (domain.ConnectedAccount, error) {
	instanceURL := strings.TrimSpace(input.InstanceURL)
	if instance != nil {
		instanceURL = instance.InstanceURL
	}

	normalizedURL, err := normalizeInstanceURL(ctx, instanceURL)
	if err != nil {
		return domain.ConnectedAccount{}, err
	}
	if strings.TrimSpace(input.AccessToken) == "" {
		return domain.ConnectedAccount{}, errors.New("access_token is required for mastodon")
	}

	return p.connectAccountWithToken(ctx, normalizedURL, providerInstanceID(instance), strings.TrimSpace(input.AccessToken), strings.TrimSpace(input.RefreshToken))
}

func (p *MastodonProvider) oauthScopesForInstance(instance domain.ProviderInstance) []string {
	scopes := append([]string(nil), instance.Scopes...)
	if len(scopes) == 0 {
		scopes = append([]string(nil), p.registration.DefaultScopes...)
	}
	if len(scopes) == 0 {
		scopes = []string{"read", "write"}
	}
	return scopes
}

func (p *MastodonProvider) BuildAuthorizationURL(instance domain.ProviderInstance, state, redirectURI string) (string, error) {
	authEndpoint := strings.TrimSpace(instance.AuthorizationEndpoint)
	if authEndpoint == "" {
		authEndpoint = strings.TrimRight(instance.InstanceURL, "/") + "/oauth/authorize"
	}
	parsed, err := url.Parse(authEndpoint)
	if err != nil {
		return "", fmt.Errorf("parse mastodon authorization endpoint: %w", err)
	}

	scopes := p.oauthScopesForInstance(instance)

	query := parsed.Query()
	query.Set("client_id", strings.TrimSpace(instance.ClientID))
	query.Set("redirect_uri", strings.TrimSpace(redirectURI))
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(scopes, " "))
	query.Set("state", strings.TrimSpace(state))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (p *MastodonProvider) ConnectAccountOAuthCallback(ctx context.Context, instance domain.ProviderInstance, clientSecret, redirectURI, code string) (domain.ConnectedAccount, error) {
	tokenEndpoint := strings.TrimSpace(instance.TokenEndpoint)
	if tokenEndpoint == "" {
		tokenEndpoint = strings.TrimRight(instance.InstanceURL, "/") + "/oauth/token"
	}

	bodyValues := url.Values{}
	bodyValues.Set("grant_type", "authorization_code")
	bodyValues.Set("code", strings.TrimSpace(code))
	bodyValues.Set("client_id", strings.TrimSpace(instance.ClientID))
	bodyValues.Set("client_secret", strings.TrimSpace(clientSecret))
	bodyValues.Set("redirect_uri", strings.TrimSpace(redirectURI))
	scopes := p.oauthScopesForInstance(instance)
	if len(scopes) > 0 {
		bodyValues.Set("scope", strings.Join(scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(bodyValues.Encode()))
	if err != nil {
		return domain.ConnectedAccount{}, fmt.Errorf("build mastodon token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return domain.ConnectedAccount{}, fmt.Errorf("exchange mastodon authorization code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		msg := strings.TrimSpace(string(errBody))
		if msg == "" {
			return domain.ConnectedAccount{}, fmt.Errorf("mastodon token exchange failed with status %d", resp.StatusCode)
		}
		return domain.ConnectedAccount{}, fmt.Errorf("mastodon token exchange failed with status %d: %s", resp.StatusCode, msg)
	}

	var tokenResponse mastodonTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return domain.ConnectedAccount{}, fmt.Errorf("decode mastodon token exchange response: %w", err)
	}
	if strings.TrimSpace(tokenResponse.AccessToken) == "" {
		return domain.ConnectedAccount{}, errors.New("mastodon token exchange returned no access token")
	}

	var accessExpires *time.Time
	if tokenResponse.ExpiresIn > 0 {
		t := time.Now().UTC().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
		accessExpires = &t
	}

	acc, err := p.connectAccountWithToken(
		ctx,
		instance.InstanceURL,
		instance.ID,
		strings.TrimSpace(tokenResponse.AccessToken),
		strings.TrimSpace(tokenResponse.RefreshToken),
	)
	if err != nil {
		return domain.ConnectedAccount{}, err
	}
	acc.AccessTokenExpiresAt = accessExpires
	return acc, nil
}

// RefreshAccessToken exchanges a refresh token for new Mastodon OAuth tokens.
func (p *MastodonProvider) RefreshAccessToken(ctx context.Context, instance domain.ProviderInstance, clientSecret, refreshToken string) (string, string, *time.Time, error) {
	tokenEndpoint := strings.TrimSpace(instance.TokenEndpoint)
	if tokenEndpoint == "" {
		tokenEndpoint = strings.TrimRight(instance.InstanceURL, "/") + "/oauth/token"
	}
	bodyValues := url.Values{}
	bodyValues.Set("grant_type", "refresh_token")
	bodyValues.Set("refresh_token", strings.TrimSpace(refreshToken))
	bodyValues.Set("client_id", strings.TrimSpace(instance.ClientID))
	bodyValues.Set("client_secret", strings.TrimSpace(clientSecret))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(bodyValues.Encode()))
	if err != nil {
		return "", "", nil, fmt.Errorf("build mastodon refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return "", "", nil, fmt.Errorf("mastodon token refresh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		msg := strings.TrimSpace(string(errBody))
		if msg == "" {
			return "", "", nil, fmt.Errorf("mastodon token refresh failed with status %d", resp.StatusCode)
		}
		return "", "", nil, fmt.Errorf("mastodon token refresh failed with status %d: %s", resp.StatusCode, msg)
	}

	var tokenResponse mastodonTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", "", nil, fmt.Errorf("decode mastodon refresh response: %w", err)
	}
	if strings.TrimSpace(tokenResponse.AccessToken) == "" {
		return "", "", nil, errors.New("mastodon refresh returned no access token")
	}
	var accessExpires *time.Time
	if tokenResponse.ExpiresIn > 0 {
		t := time.Now().UTC().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
		accessExpires = &t
	}
	newRT := strings.TrimSpace(tokenResponse.RefreshToken)
	return strings.TrimSpace(tokenResponse.AccessToken), newRT, accessExpires, nil
}

func (p *MastodonProvider) connectAccountWithToken(ctx context.Context, normalizedURL, providerInstanceID, accessToken, refreshToken string) (domain.ConnectedAccount, error) {
	resp, err := doJSONRequest(ctx, http.MethodGet, normalizedURL+"/api/v1/accounts/verify_credentials", accessToken, nil)
	if err != nil {
		return domain.ConnectedAccount{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return domain.ConnectedAccount{}, fmt.Errorf("mastodon account verification failed with status %d", resp.StatusCode)
	}

	var payload mastodonAccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return domain.ConnectedAccount{}, fmt.Errorf("decode mastodon account response: %w", err)
	}

	username := strings.TrimSpace(payload.Acct)
	if username == "" {
		username = strings.TrimSpace(payload.Username)
	}

	avatarURL := strings.TrimSpace(payload.AvatarStatic)
	if avatarURL == "" {
		avatarURL = strings.TrimSpace(payload.Avatar)
	}

	return domain.ConnectedAccount{
		Provider:           p.Name(),
		AuthType:           domain.AccountAuthTypeOAuthToken,
		ProviderInstanceID: providerInstanceID,
		InstanceURL:        normalizedURL,
		Username:           username,
		RemoteAccountID:    strings.TrimSpace(payload.ID),
		AvatarURL:          avatarURL,
		AccessToken:        accessToken,
		RefreshToken:       refreshToken,
	}, nil
}

func (p *MastodonProvider) UploadMedia(ctx context.Context, account domain.SocialAccount, auth PublishAuth, file io.Reader, filename, mimeType, altText string) (string, error) {
	return uploadMastodonV2Media(ctx, strings.TrimRight(account.InstanceURL, "/"), auth.AccessToken, file, filename, mimeType, altText)
}

func (p *MastodonProvider) Publish(ctx context.Context, account domain.SocialAccount, auth PublishAuth, req PublishRequest) (PublishResult, error) {
	body, err := marshalJSONBody(mastodonCompatibleStatusPayload(req))
	if err != nil {
		return PublishResult{}, err
	}

	resp, err := doJSONRequest(ctx, http.MethodPost, strings.TrimRight(account.InstanceURL, "/")+"/api/v1/statuses", auth.AccessToken, body)
	if err != nil {
		return PublishResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return PublishResult{}, fmt.Errorf("mastodon publish failed with status %d", resp.StatusCode)
	}

	var statusPayload mastodonStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusPayload); err != nil {
		return PublishResult{}, fmt.Errorf("decode mastodon publish response: %w", err)
	}

	meta := map[string]string{}
	if strings.TrimSpace(statusPayload.URI) != "" {
		meta["uri"] = strings.TrimSpace(statusPayload.URI)
	}
	return PublishResult{
		RemoteID: statusPayload.ID,
		URL:      statusPayload.URL,
		Metadata: meta,
	}, nil
}

func (p *MastodonProvider) GetMetrics(ctx context.Context, account domain.SocialAccount, auth PublishAuth, publishedURL string) ([]EngagementMetric, error) {
	return fetchMastodonCompatibleMetrics(ctx, account.InstanceURL, auth.AccessToken, "/api/v1/statuses", publishedURL)
}

func (p *MastodonProvider) GetAccountMetrics(ctx context.Context, account domain.SocialAccount, auth PublishAuth) ([]AccountMetric, error) {
	resp, err := doJSONRequest(ctx, http.MethodGet, strings.TrimRight(account.InstanceURL, "/")+"/api/v1/accounts/verify_credentials", auth.AccessToken, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("mastodon account metrics failed with status %d", resp.StatusCode)
	}
	var payload mastodonAccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode mastodon account metrics response: %w", err)
	}
	return []AccountMetric{
		{Name: "followers", Value: payload.FollowersCount},
		{Name: "following", Value: payload.FollowingCount},
		{Name: "posts", Value: payload.StatusesCount},
	}, nil
}

func discoverMastodonOAuthMetadata(ctx context.Context, instanceURL string) (string, string, error) {
	resp, err := doJSONRequest(ctx, http.MethodGet, strings.TrimRight(instanceURL, "/")+"/.well-known/oauth-authorization-server", "", nil)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusBadRequest {
			var payload mastodonAuthorizationServerMetadata
			if decodeErr := json.NewDecoder(resp.Body).Decode(&payload); decodeErr == nil {
				authEndpoint := strings.TrimSpace(payload.AuthorizationEndpoint)
				tokenEndpoint := strings.TrimSpace(payload.TokenEndpoint)
				if authEndpoint != "" && tokenEndpoint != "" {
					return authEndpoint, tokenEndpoint, nil
				}
			}
		}
	}

	baseURL := strings.TrimRight(instanceURL, "/")
	return baseURL + "/oauth/authorize", baseURL + "/oauth/token", nil
}

func (p *MastodonProvider) registerApplication(ctx context.Context, instanceURL string, scopes []string) (mastodonAppRegistrationResponse, error) {
	body, err := marshalJSONBody(mastodonAppRegistrationRequest{
		ClientName:   strings.TrimSpace(p.registration.AppName),
		RedirectURIs: []string{strings.TrimSpace(p.registration.RedirectURI)},
		Scopes:       strings.Join(scopes, " "),
		Website:      strings.TrimSpace(p.registration.Website),
	})
	if err != nil {
		return mastodonAppRegistrationResponse{}, err
	}

	resp, err := doJSONRequest(ctx, http.MethodPost, strings.TrimRight(instanceURL, "/")+"/api/v1/apps", "", body)
	if err != nil {
		return mastodonAppRegistrationResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return mastodonAppRegistrationResponse{}, fmt.Errorf("mastodon app registration failed with status %d", resp.StatusCode)
	}

	var payload mastodonAppRegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return mastodonAppRegistrationResponse{}, fmt.Errorf("decode mastodon app registration response: %w", err)
	}
	if strings.TrimSpace(payload.ClientID) == "" || strings.TrimSpace(payload.ClientSecret) == "" {
		return mastodonAppRegistrationResponse{}, errors.New("mastodon app registration returned incomplete credentials")
	}
	return payload, nil
}
