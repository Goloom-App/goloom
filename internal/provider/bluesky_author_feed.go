package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

type blueskyAuthorFeedResponse struct {
	Feed []blueskyFeedViewPost `json:"feed"`
}

type blueskyFeedViewPost struct {
	Post   blueskyFeedPost `json:"post"`
	Reason json.RawMessage `json:"reason"`
}

type blueskyFeedPost struct {
	URI    string `json:"uri"`
	Record struct {
		Text      string `json:"text"`
		CreatedAt string `json:"createdAt"`
		Reply     any    `json:"reply"`
	} `json:"record"`
}

func (p *BlueskyProvider) blueskyAccessToken(ctx context.Context, account domain.SocialAccount, auth PublishAuth) (string, error) {
	token := strings.TrimSpace(auth.AccessToken)
	if token == "" {
		return "", fmt.Errorf("missing bluesky credential")
	}
	if account.AuthType == domain.AccountAuthTypeAppPassword {
		session, err := p.createSession(ctx, account.InstanceURL, account.Username, token)
		if err != nil {
			return "", err
		}
		token = session.AccessJWT
	}
	return token, nil
}

func (p *BlueskyProvider) ListAuthorPosts(ctx context.Context, account domain.SocialAccount, auth PublishAuth, since time.Time, limit int) ([]AuthorPost, error) {
	actor := strings.TrimSpace(account.RemoteAccountID)
	if actor == "" {
		return nil, fmt.Errorf("remote account id (did) is required")
	}
	token, err := p.blueskyAccessToken(ctx, account, auth)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 40
	}
	if limit > 100 {
		limit = 100
	}

	q := url.Values{}
	q.Set("actor", actor)
	q.Set("limit", fmt.Sprintf("%d", limit))
	endpoint := strings.TrimRight(account.InstanceURL, "/") + "/xrpc/app.bsky.feed.getAuthorFeed?" + q.Encode()

	resp, err := doJSONRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("bluesky author feed failed with status %d", resp.StatusCode)
	}

	var payload blueskyAuthorFeedResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode bluesky author feed: %w", err)
	}

	out := make([]AuthorPost, 0, len(payload.Feed))
	for _, item := range payload.Feed {
		if len(item.Reason) > 0 && string(item.Reason) != "null" {
			continue
		}
		if item.Post.Record.Reply != nil {
			continue
		}
		uri := strings.TrimSpace(item.Post.URI)
		if uri == "" {
			continue
		}
		publishedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(item.Post.Record.CreatedAt))
		if err != nil {
			continue
		}
		publishedAt = publishedAt.UTC()
		if !since.IsZero() && publishedAt.Before(since) {
			continue
		}
		out = append(out, AuthorPost{
			RemoteID:    uri,
			URL:         buildBlueskyPostURL(account.Username, uri),
			Content:     strings.TrimSpace(item.Post.Record.Text),
			PublishedAt: publishedAt,
			Metadata:    map[string]string{"uri": uri},
		})
	}
	return out, nil
}
