package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

type mastodonAuthorStatus struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	URI       string `json:"uri"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

func stripHTMLContent(raw string) string {
	s := htmlTagRe.ReplaceAllString(raw, "")
	return strings.TrimSpace(html.UnescapeString(s))
}

func fetchMastodonCompatibleAuthorPosts(ctx context.Context, instanceURL, accessToken, accountID string, since time.Time, limit int) ([]AuthorPost, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("remote account id is required")
	}
	if strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("access token is required")
	}
	if limit <= 0 {
		limit = 40
	}
	if limit > 80 {
		limit = 80
	}

	q := url.Values{}
	q.Set("exclude_replies", "true")
	q.Set("exclude_reblogs", "true")
	q.Set("limit", fmt.Sprintf("%d", limit))
	endpoint := strings.TrimRight(strings.TrimSpace(instanceURL), "/") +
		"/api/v1/accounts/" + url.PathEscape(accountID) + "/statuses?" + q.Encode()

	resp, err := doJSONRequest(ctx, http.MethodGet, endpoint, accessToken, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("author feed failed with status %d", resp.StatusCode)
	}

	var statuses []mastodonAuthorStatus
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, fmt.Errorf("decode author feed: %w", err)
	}

	out := make([]AuthorPost, 0, len(statuses))
	for _, st := range statuses {
		publishedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(st.CreatedAt))
		if err != nil {
			continue
		}
		publishedAt = publishedAt.UTC()
		if !since.IsZero() && publishedAt.Before(since) {
			continue
		}
		remoteID := strings.TrimSpace(st.ID)
		if remoteID == "" {
			continue
		}
		meta := map[string]string{}
		if uri := strings.TrimSpace(st.URI); uri != "" {
			meta["uri"] = uri
		}
		out = append(out, AuthorPost{
			RemoteID:    remoteID,
			URL:         strings.TrimSpace(st.URL),
			Content:     stripHTMLContent(st.Content),
			PublishedAt: publishedAt,
			Metadata:    meta,
		})
	}
	return out, nil
}

func (p *MastodonProvider) ListAuthorPosts(ctx context.Context, account domain.SocialAccount, auth PublishAuth, since time.Time, limit int) ([]AuthorPost, error) {
	return fetchMastodonCompatibleAuthorPosts(ctx, account.InstanceURL, auth.AccessToken, account.RemoteAccountID, since, limit)
}

func (p *GenericStatusProvider) ListAuthorPosts(ctx context.Context, account domain.SocialAccount, auth PublishAuth, since time.Time, limit int) ([]AuthorPost, error) {
	return fetchMastodonCompatibleAuthorPosts(ctx, account.InstanceURL, auth.AccessToken, account.RemoteAccountID, since, limit)
}
