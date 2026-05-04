package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var mastodonNumericID = regexp.MustCompile(`^\d+$`)

type mastodonStatusCounts struct {
	FavouritesCount int `json:"favourites_count"`
	ReblogsCount    int `json:"reblogs_count"`
	RepliesCount    int `json:"replies_count"`
}

func mastodonStatusIDFromPublishedURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(segs)-1; i++ {
		if strings.EqualFold(segs[i], "statuses") {
			id := strings.TrimSpace(segs[i+1])
			if id != "" {
				return id, nil
			}
		}
	}
	for i := len(segs) - 1; i >= 0; i-- {
		s := strings.TrimSpace(segs[i])
		if s == "" {
			continue
		}
		if mastodonNumericID.MatchString(s) {
			return s, nil
		}
	}
	return "", fmt.Errorf("mastodon-style status id not found in url")
}

func fetchMastodonCompatibleMetrics(ctx context.Context, instanceBaseURL, accessToken, statusGETPath, publishedURL string) ([]EngagementMetric, error) {
	if strings.TrimSpace(publishedURL) == "" {
		return nil, fmt.Errorf("published url is required for metrics")
	}
	if strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("access token is required for metrics")
	}
	id, err := mastodonStatusIDFromPublishedURL(publishedURL)
	if err != nil {
		return nil, err
	}

	base := strings.TrimRight(strings.TrimSpace(instanceBaseURL), "/")
	path := strings.TrimSpace(statusGETPath)
	if path == "" {
		path = "/api/v1/statuses"
	}
	endpoint := base + path + "/" + url.PathEscape(id)

	resp, err := doJSONRequest(ctx, http.MethodGet, endpoint, accessToken, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("status fetch failed with status %d", resp.StatusCode)
	}

	var payload mastodonStatusCounts
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}

	return []EngagementMetric{
		{Name: "likes", Value: int64(payload.FavouritesCount)},
		{Name: "reposts", Value: int64(payload.ReblogsCount)},
		{Name: "replies", Value: int64(payload.RepliesCount)},
	}, nil
}
