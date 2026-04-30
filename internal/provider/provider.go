package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
)

type Capabilities struct {
	MaxChars   int      `json:"max_chars"`
	MediaTypes []string `json:"media_types"`
}

type PublishRequest struct {
	Content string
}

type PublishResult struct {
	RemoteID string
	URL      string
}

type SocialMediaProvider interface {
	Name() string
	Capabilities(ctx context.Context, account domain.SocialAccount) (Capabilities, error)
	Publish(ctx context.Context, account domain.SocialAccount, accessToken string, req PublishRequest) (PublishResult, error)
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
	return out
}

type HTTPClientProvider struct {
	name         string
	defaultChars int
	mediaTypes   []string
	postPath     string
}

func NewMastodonProvider() SocialMediaProvider {
	return &HTTPClientProvider{
		name:         "mastodon",
		defaultChars: 500,
		mediaTypes:   []string{"image/jpeg", "image/png", "video/mp4"},
		postPath:     "/api/v1/statuses",
	}
}

func NewFriendicaProvider() SocialMediaProvider {
	return &HTTPClientProvider{
		name:         "friendica",
		defaultChars: 5000,
		mediaTypes:   []string{"image/jpeg", "image/png", "video/mp4"},
		postPath:     "/api/v1/statuses",
	}
}

func NewBlueskyProvider() SocialMediaProvider {
	return &HTTPClientProvider{
		name:         "bluesky",
		defaultChars: 300,
		mediaTypes:   []string{"image/jpeg", "image/png"},
		postPath:     "/xrpc/com.atproto.repo.createRecord",
	}
}

func (p *HTTPClientProvider) Name() string {
	return p.name
}

func (p *HTTPClientProvider) Capabilities(_ context.Context, account domain.SocialAccount) (Capabilities, error) {
	if account.MaxCharsOverride != nil {
		return Capabilities{MaxChars: *account.MaxCharsOverride, MediaTypes: p.mediaTypes}, nil
	}
	return Capabilities{MaxChars: p.defaultChars, MediaTypes: p.mediaTypes}, nil
}

func (p *HTTPClientProvider) Publish(ctx context.Context, account domain.SocialAccount, accessToken string, req PublishRequest) (PublishResult, error) {
	payload := map[string]any{
		"text":   req.Content,
		"status": req.Content,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return PublishResult{}, fmt.Errorf("marshal publish request: %w", err)
	}

	url := strings.TrimRight(account.InstanceURL, "/") + p.postPath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return PublishResult{}, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return PublishResult{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return PublishResult{}, fmt.Errorf("%s publish failed with status %d", p.name, resp.StatusCode)
	}

	return PublishResult{
		RemoteID: fmt.Sprintf("%s:%s", p.name, account.ID),
		URL:      account.InstanceURL,
	}, nil
}
