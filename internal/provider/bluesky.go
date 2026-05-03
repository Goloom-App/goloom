package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

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

// BlueskyProvider implements SocialMediaProvider for ATProto / Bluesky.
type BlueskyProvider struct {
	defaultChars int
	mediaTypes   []string
}

// NewBlueskyProvider returns a Bluesky integration with default PDS assumptions.
func NewBlueskyProvider() SocialMediaProvider {
	return &BlueskyProvider{
		defaultChars: 300,
		mediaTypes:   []string{"image/jpeg", "image/png"},
	}
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

		avatar := p.fetchBlueskyActorAvatar(ctx, normalizedURL, session.AccessJWT, session.Handle)
		if avatar == "" {
			avatar = p.fetchBlueskyActorAvatar(ctx, normalizedURL, session.AccessJWT, session.DID)
		}

		return domain.ConnectedAccount{
			Provider:           p.Name(),
			AuthType:           domain.AccountAuthTypeAppPassword,
			ProviderInstanceID: providerInstanceID(instance),
			InstanceURL:        normalizedURL,
			Username:           session.Handle,
			RemoteAccountID:    session.DID,
			AvatarURL:          avatar,
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

	token := strings.TrimSpace(input.AccessToken)
	avatar := p.fetchBlueskyActorAvatar(ctx, normalizedURL, token, session.Handle)
	if avatar == "" {
		avatar = p.fetchBlueskyActorAvatar(ctx, normalizedURL, token, session.DID)
	}

	return domain.ConnectedAccount{
		Provider:           p.Name(),
		AuthType:           domain.AccountAuthTypeOAuthToken,
		ProviderInstanceID: providerInstanceID(instance),
		InstanceURL:        normalizedURL,
		Username:           session.Handle,
		RemoteAccountID:    session.DID,
		AvatarURL:          avatar,
		AccessToken:        token,
		RefreshToken:       strings.TrimSpace(input.RefreshToken),
	}, nil
}

func (p *BlueskyProvider) fetchBlueskyActorAvatar(ctx context.Context, instanceURL, bearerJWT, actor string) string {
	bearerJWT = strings.TrimSpace(bearerJWT)
	actor = strings.TrimSpace(actor)
	if bearerJWT == "" || actor == "" {
		return ""
	}
	endpoint := strings.TrimRight(instanceURL, "/") + "/xrpc/app.bsky.actor.getProfile?actor=" + url.QueryEscape(actor)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+bearerJWT)
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return ""
	}
	var out struct {
		Avatar *string `json:"avatar"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ""
	}
	if out.Avatar == nil {
		return ""
	}
	return strings.TrimSpace(*out.Avatar)
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
