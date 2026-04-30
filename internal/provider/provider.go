package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

var defaultHTTPClient = &http.Client{Timeout: 15 * time.Second}

type Capabilities struct {
	MaxChars   int      `json:"max_chars"`
	MediaTypes []string `json:"media_types"`
}

type PublishRequest struct {
	Content string
}

type PublishAuth struct {
	AccessToken  string
	RefreshToken string
}

type PublishResult struct {
	RemoteID string
	URL      string
}

type OAuthAccountConnector interface {
	BuildAuthorizationURL(instance domain.ProviderInstance, state, redirectURI string) (string, error)
	ConnectAccountOAuthCallback(ctx context.Context, instance domain.ProviderInstance, clientSecret, redirectURI, code string) (domain.ConnectedAccount, error)
}

type SocialMediaProvider interface {
	Name() string
	Capabilities(ctx context.Context, account domain.SocialAccount) (Capabilities, error)
	PrepareProviderInstance(ctx context.Context, input domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error)
	ConnectAccount(ctx context.Context, input domain.CreateAccountInput, instance *domain.ProviderInstance) (domain.ConnectedAccount, error)
	Publish(ctx context.Context, account domain.SocialAccount, auth PublishAuth, req PublishRequest) (PublishResult, error)
}

type Registry struct {
	providers map[string]SocialMediaProvider
}

func NewRegistry(providers ...SocialMediaProvider) *Registry {
	items := make(map[string]SocialMediaProvider, len(providers))
	for _, item := range providers {
		items[item.Name()] = item
	}
	return &Registry{providers: items}
}

func (r *Registry) Get(name string) (SocialMediaProvider, bool) {
	provider, ok := r.providers[strings.ToLower(name)]
	return provider, ok
}

func (r *Registry) Supported() []string {
	out := make([]string, 0, len(r.providers))
	for name := range r.providers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type GenericStatusProvider struct {
	name         string
	defaultChars int
	mediaTypes   []string
	postPath     string
}

type MastodonProvider struct {
	defaultChars int
	mediaTypes   []string
	registration MastodonRegistrationConfig
}

type BlueskyProvider struct {
	defaultChars int
	mediaTypes   []string
}

type mastodonAccountResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Acct     string `json:"acct"`
	URL      string `json:"url"`
}

type mastodonStatusResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type mastodonAuthorizationServerMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

type mastodonTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
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

type MastodonRegistrationConfig struct {
	AppName       string
	RedirectURI   string
	Website       string
	DefaultScopes []string
}

type blueskySessionResponse struct {
	DID        string `json:"did"`
	Handle     string `json:"handle"`
	AccessJWT  string `json:"accessJwt"`
	RefreshJWT string `json:"refreshJwt"`
}

type blueskyCreateRecordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

func NewMastodonProvider(cfg MastodonRegistrationConfig) SocialMediaProvider {
	if strings.TrimSpace(cfg.AppName) == "" {
		cfg.AppName = "goloom"
	}
	if strings.TrimSpace(cfg.RedirectURI) == "" {
		cfg.RedirectURI = "urn:ietf:wg:oauth:2.0:oob"
	}
	if len(cfg.DefaultScopes) == 0 {
		cfg.DefaultScopes = []string{"read", "write"}
	}
	return &MastodonProvider{
		defaultChars: 500,
		mediaTypes:   []string{"image/jpeg", "image/png", "video/mp4"},
		registration: cfg,
	}
}

func NewFriendicaProvider() SocialMediaProvider {
	return &GenericStatusProvider{
		name:         "friendica",
		defaultChars: 5000,
		mediaTypes:   []string{"image/jpeg", "image/png", "video/mp4"},
		postPath:     "/api/v1/statuses",
	}
}

func NewBlueskyProvider() SocialMediaProvider {
	return &BlueskyProvider{
		defaultChars: 300,
		mediaTypes:   []string{"image/jpeg", "image/png"},
	}
}

func (p *GenericStatusProvider) Name() string {
	return p.name
}

func (p *GenericStatusProvider) Capabilities(_ context.Context, account domain.SocialAccount) (Capabilities, error) {
	return capabilitiesForAccount(account, p.defaultChars, p.mediaTypes), nil
}

func (p *GenericStatusProvider) PrepareProviderInstance(_ context.Context, input domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error) {
	instanceURL, err := normalizeInstanceURL(input.InstanceURL)
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

func (p *GenericStatusProvider) ConnectAccount(_ context.Context, input domain.CreateAccountInput, instance *domain.ProviderInstance) (domain.ConnectedAccount, error) {
	instanceURL := strings.TrimSpace(input.InstanceURL)
	if instance != nil {
		instanceURL = instance.InstanceURL
	}
	normalizedURL, err := normalizeInstanceURL(instanceURL)
	if err != nil {
		return domain.ConnectedAccount{}, err
	}
	if strings.TrimSpace(input.Username) == "" || strings.TrimSpace(input.AccessToken) == "" {
		return domain.ConnectedAccount{}, errors.New("username and access_token are required")
	}
	return domain.ConnectedAccount{
		Provider:           p.name,
		AuthType:           domain.AccountAuthTypeOAuthToken,
		ProviderInstanceID: providerInstanceID(instance),
		InstanceURL:        normalizedURL,
		Username:           strings.TrimSpace(input.Username),
		RemoteAccountID:    strings.TrimSpace(input.RemoteAccountID),
		AccessToken:        strings.TrimSpace(input.AccessToken),
		RefreshToken:       strings.TrimSpace(input.RefreshToken),
	}, nil
}

func (p *GenericStatusProvider) Publish(ctx context.Context, account domain.SocialAccount, auth PublishAuth, req PublishRequest) (PublishResult, error) {
	body, err := marshalJSONBody(map[string]any{"status": req.Content})
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

	var payload mastodonStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return PublishResult{}, fmt.Errorf("decode %s publish response: %w", p.name, err)
	}

	return PublishResult{RemoteID: payload.ID, URL: payload.URL}, nil
}

func (p *MastodonProvider) Name() string {
	return "mastodon"
}

func (p *MastodonProvider) Capabilities(_ context.Context, account domain.SocialAccount) (Capabilities, error) {
	return capabilitiesForAccount(account, p.defaultChars, p.mediaTypes), nil
}

func (p *MastodonProvider) PrepareProviderInstance(ctx context.Context, input domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error) {
	instanceURL, err := normalizeInstanceURL(input.InstanceURL)
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

	normalizedURL, err := normalizeInstanceURL(instanceURL)
	if err != nil {
		return domain.ConnectedAccount{}, err
	}
	if strings.TrimSpace(input.AccessToken) == "" {
		return domain.ConnectedAccount{}, errors.New("access_token is required for mastodon")
	}

	return p.connectAccountWithToken(ctx, normalizedURL, providerInstanceID(instance), strings.TrimSpace(input.AccessToken), strings.TrimSpace(input.RefreshToken))
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

	scopes := instance.Scopes
	if len(scopes) == 0 {
		scopes = append([]string(nil), p.registration.DefaultScopes...)
	}

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
	if len(instance.Scopes) > 0 {
		bodyValues.Set("scope", strings.Join(instance.Scopes, " "))
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
		return domain.ConnectedAccount{}, fmt.Errorf("mastodon token exchange failed with status %d", resp.StatusCode)
	}

	var tokenResponse mastodonTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return domain.ConnectedAccount{}, fmt.Errorf("decode mastodon token exchange response: %w", err)
	}
	if strings.TrimSpace(tokenResponse.AccessToken) == "" {
		return domain.ConnectedAccount{}, errors.New("mastodon token exchange returned no access token")
	}

	return p.connectAccountWithToken(
		ctx,
		instance.InstanceURL,
		instance.ID,
		strings.TrimSpace(tokenResponse.AccessToken),
		strings.TrimSpace(tokenResponse.RefreshToken),
	)
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

	return domain.ConnectedAccount{
		Provider:           p.Name(),
		AuthType:           domain.AccountAuthTypeOAuthToken,
		ProviderInstanceID: providerInstanceID,
		InstanceURL:        normalizedURL,
		Username:           username,
		RemoteAccountID:    strings.TrimSpace(payload.ID),
		AccessToken:        accessToken,
		RefreshToken:       refreshToken,
	}, nil
}

func (p *MastodonProvider) Publish(ctx context.Context, account domain.SocialAccount, auth PublishAuth, req PublishRequest) (PublishResult, error) {
	body, err := marshalJSONBody(map[string]any{"status": req.Content})
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

	var payload mastodonStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return PublishResult{}, fmt.Errorf("decode mastodon publish response: %w", err)
	}

	return PublishResult{
		RemoteID: payload.ID,
		URL:      payload.URL,
	}, nil
}

func (p *BlueskyProvider) Name() string {
	return "bluesky"
}

func (p *BlueskyProvider) Capabilities(_ context.Context, account domain.SocialAccount) (Capabilities, error) {
	return capabilitiesForAccount(account, p.defaultChars, p.mediaTypes), nil
}

func (p *BlueskyProvider) PrepareProviderInstance(_ context.Context, input domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error) {
	instanceURL := strings.TrimSpace(input.InstanceURL)
	if instanceURL == "" {
		instanceURL = "https://bsky.social"
	}

	normalizedURL, err := normalizeInstanceURL(instanceURL)
	if err != nil {
		return domain.PreparedProviderInstance{}, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = strings.TrimPrefix(strings.TrimPrefix(normalizedURL, "https://"), "http://")
	}

	return domain.PreparedProviderInstance{
		Provider:              p.Name(),
		Name:                  name,
		InstanceURL:           normalizedURL,
		ClientID:              strings.TrimSpace(input.ClientID),
		ClientSecret:          strings.TrimSpace(input.ClientSecret),
		Scopes:                cleanScopes(input.Scopes),
		AuthorizationEndpoint: strings.TrimSpace(input.AuthorizationEndpoint),
		TokenEndpoint:         strings.TrimSpace(input.TokenEndpoint),
	}, nil
}

func (p *BlueskyProvider) ConnectAccount(ctx context.Context, input domain.CreateAccountInput, instance *domain.ProviderInstance) (domain.ConnectedAccount, error) {
	instanceURL := strings.TrimSpace(input.InstanceURL)
	if instance != nil {
		instanceURL = instance.InstanceURL
	}
	if instanceURL == "" {
		instanceURL = "https://bsky.social"
	}

	normalizedURL, err := normalizeInstanceURL(instanceURL)
	if err != nil {
		return domain.ConnectedAccount{}, err
	}

	if strings.TrimSpace(input.AppPassword) != "" {
		identifier := strings.TrimSpace(input.Identifier)
		if identifier == "" {
			identifier = strings.TrimSpace(input.Username)
		}
		if identifier == "" {
			return domain.ConnectedAccount{}, errors.New("identifier or username is required for bluesky app password auth")
		}

		session, err := p.createSession(ctx, normalizedURL, identifier, strings.TrimSpace(input.AppPassword))
		if err != nil {
			return domain.ConnectedAccount{}, err
		}

		return domain.ConnectedAccount{
			Provider:           p.Name(),
			AuthType:           domain.AccountAuthTypeAppPassword,
			ProviderInstanceID: providerInstanceID(instance),
			InstanceURL:        normalizedURL,
			Username:           session.Handle,
			RemoteAccountID:    session.DID,
			AccessToken:        strings.TrimSpace(input.AppPassword),
		}, nil
	}

	if strings.TrimSpace(input.AccessToken) == "" {
		return domain.ConnectedAccount{}, errors.New("access_token or app_password is required for bluesky")
	}

	resp, err := doJSONRequest(ctx, http.MethodGet, normalizedURL+"/xrpc/com.atproto.server.getSession", strings.TrimSpace(input.AccessToken), nil)
	if err != nil {
		return domain.ConnectedAccount{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return domain.ConnectedAccount{}, fmt.Errorf("bluesky session verification failed with status %d", resp.StatusCode)
	}

	var session blueskySessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return domain.ConnectedAccount{}, fmt.Errorf("decode bluesky session response: %w", err)
	}

	return domain.ConnectedAccount{
		Provider:           p.Name(),
		AuthType:           domain.AccountAuthTypeOAuthToken,
		ProviderInstanceID: providerInstanceID(instance),
		InstanceURL:        normalizedURL,
		Username:           session.Handle,
		RemoteAccountID:    session.DID,
		AccessToken:        strings.TrimSpace(input.AccessToken),
		RefreshToken:       strings.TrimSpace(input.RefreshToken),
	}, nil
}

func (p *BlueskyProvider) Publish(ctx context.Context, account domain.SocialAccount, auth PublishAuth, req PublishRequest) (PublishResult, error) {
	token := strings.TrimSpace(auth.AccessToken)
	if token == "" {
		return PublishResult{}, errors.New("missing bluesky credential")
	}

	if account.AuthType == domain.AccountAuthTypeAppPassword {
		session, err := p.createSession(ctx, account.InstanceURL, account.Username, token)
		if err != nil {
			return PublishResult{}, err
		}
		token = session.AccessJWT
	}

	body, err := marshalJSONBody(map[string]any{
		"repo":       account.RemoteAccountID,
		"collection": "app.bsky.feed.post",
		"record": map[string]any{
			"$type":     "app.bsky.feed.post",
			"text":      req.Content,
			"createdAt": time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		return PublishResult{}, err
	}

	resp, err := doJSONRequest(ctx, http.MethodPost, strings.TrimRight(account.InstanceURL, "/")+"/xrpc/com.atproto.repo.createRecord", token, body)
	if err != nil {
		return PublishResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return PublishResult{}, fmt.Errorf("bluesky publish failed with status %d", resp.StatusCode)
	}

	var payload blueskyCreateRecordResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return PublishResult{}, fmt.Errorf("decode bluesky publish response: %w", err)
	}

	return PublishResult{
		RemoteID: payload.URI,
		URL:      buildBlueskyPostURL(account.Username, payload.URI),
	}, nil
}

func (p *BlueskyProvider) createSession(ctx context.Context, instanceURL, identifier, password string) (blueskySessionResponse, error) {
	body, err := marshalJSONBody(map[string]any{
		"identifier": identifier,
		"password":   password,
	})
	if err != nil {
		return blueskySessionResponse{}, err
	}

	resp, err := doJSONRequest(ctx, http.MethodPost, strings.TrimRight(instanceURL, "/")+"/xrpc/com.atproto.server.createSession", "", body)
	if err != nil {
		return blueskySessionResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return blueskySessionResponse{}, fmt.Errorf("bluesky session creation failed with status %d", resp.StatusCode)
	}

	var payload blueskySessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return blueskySessionResponse{}, fmt.Errorf("decode bluesky session response: %w", err)
	}
	return payload, nil
}

func capabilitiesForAccount(account domain.SocialAccount, defaultChars int, mediaTypes []string) Capabilities {
	if account.MaxCharsOverride != nil {
		return Capabilities{MaxChars: *account.MaxCharsOverride, MediaTypes: mediaTypes}
	}
	return Capabilities{MaxChars: defaultChars, MediaTypes: mediaTypes}
}

func normalizeInstanceURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("instance_url is required")
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse instance_url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("instance_url must include a valid host")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func marshalJSONBody(payload any) (*bytes.Reader, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}
	return bytes.NewReader(body), nil
}

func doJSONRequest(ctx context.Context, method, endpoint, bearerToken string, body *bytes.Reader) (*http.Response, error) {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = body
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	return resp, nil
}

func buildBlueskyPostURL(handle, uri string) string {
	trimmedHandle := strings.TrimSpace(handle)
	if trimmedHandle == "" || uri == "" {
		return ""
	}
	parts := strings.Split(strings.TrimSpace(uri), "/")
	if len(parts) == 0 {
		return ""
	}
	rkey := parts[len(parts)-1]
	if rkey == "" {
		return ""
	}
	return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", trimmedHandle, rkey)
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

func cleanScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		normalized := strings.TrimSpace(scope)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func providerInstanceID(instance *domain.ProviderInstance) string {
	if instance == nil {
		return ""
	}
	return instance.ID
}
