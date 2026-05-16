package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const blueskyPublicAppView = "https://public.api.bsky.app"

type blueskyPostViewCounts struct {
	LikeCount   int64 `json:"likeCount"`
	RepostCount int64 `json:"repostCount"`
	ReplyCount  int64 `json:"replyCount"`
	QuoteCount  int64 `json:"quoteCount"`
}

func parseBlueskyGetPostsResponse(body []byte) (blueskyPostViewCounts, error) {
	var envelope struct {
		Posts []json.RawMessage `json:"posts"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return blueskyPostViewCounts{}, fmt.Errorf("decode getPosts: %w", err)
	}
	if len(envelope.Posts) == 0 {
		return blueskyPostViewCounts{}, fmt.Errorf("getPosts returned no post")
	}

	raw := envelope.Posts[0]
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return blueskyPostViewCounts{}, fmt.Errorf("decode getPosts item: %w", err)
	}

	var outer blueskyPostViewCounts
	if err := json.Unmarshal(raw, &outer); err != nil {
		return blueskyPostViewCounts{}, fmt.Errorf("decode getPosts postView: %w", err)
	}

	nested, hasNestedPost := probe["post"]
	if !hasNestedPost {
		return outer, nil
	}

	var inner blueskyPostViewCounts
	if err := json.Unmarshal(nested, &inner); err != nil {
		return blueskyPostViewCounts{}, fmt.Errorf("decode getPosts nested post: %w", err)
	}
	// feedViewPost wraps a postView; some gateways still expose counts on the outer object.
	if inner.LikeCount == 0 && inner.RepostCount == 0 && inner.ReplyCount == 0 && inner.QuoteCount == 0 {
		if outer.LikeCount != 0 || outer.RepostCount != 0 || outer.ReplyCount != 0 || outer.QuoteCount != 0 {
			return outer, nil
		}
	}
	return inner, nil
}

func blueskyFetchPostCounts(ctx context.Context, baseURL, bearerJWT, atURI string) (blueskyPostViewCounts, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	atURI = strings.TrimSpace(atURI)
	if baseURL == "" || atURI == "" {
		return blueskyPostViewCounts{}, fmt.Errorf("bluesky getPosts missing base url or at-uri")
	}

	q := url.Values{}
	q.Set("uris", atURI)
	endpoint := baseURL + "/xrpc/app.bsky.feed.getPosts?" + q.Encode()

	resp, err := doJSONRequest(ctx, http.MethodGet, endpoint, strings.TrimSpace(bearerJWT), nil)
	if err != nil {
		return blueskyPostViewCounts{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return blueskyPostViewCounts{}, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return blueskyPostViewCounts{}, fmt.Errorf("bluesky getPosts failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return parseBlueskyGetPostsResponse(raw)
}

func blueskyEngagementFromCounts(counts blueskyPostViewCounts) []EngagementMetric {
	return []EngagementMetric{
		{Name: "likes", Value: counts.LikeCount},
		{Name: "reposts", Value: counts.RepostCount},
		{Name: "replies", Value: counts.ReplyCount},
		{Name: "quotes", Value: counts.QuoteCount},
	}
}
