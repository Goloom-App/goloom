package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

const (
	// maxThreadReplies caps how many replies are returned to API callers.
	maxThreadReplies = 200
	// maxThreadBodyBytes bounds the context/search response read from the remote instance.
	maxThreadBodyBytes = 4 << 20
)

var (
	htmlBreakRe     = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlParagraphRe = regexp.MustCompile(`(?i)</p>\s*<p[^>]*>`)
)

// htmlToPlainText converts status HTML to text, keeping line breaks (<br> -> \n, paragraph break -> blank line).
func htmlToPlainText(raw string) string {
	s := htmlBreakRe.ReplaceAllString(raw, "\n")
	s = htmlParagraphRe.ReplaceAllString(s, "\n\n")
	return stripHTMLContent(s)
}

type mastodonThreadAccount struct {
	Acct         string `json:"acct"`
	DisplayName  string `json:"display_name"`
	Avatar       string `json:"avatar"`
	AvatarStatic string `json:"avatar_static"`
	URL          string `json:"url"`
}

type mastodonThreadStatus struct {
	mastodonAuthorStatus
	InReplyToID string                `json:"in_reply_to_id"`
	Visibility  string                `json:"visibility"`
	Account     mastodonThreadAccount `json:"account"`
}

// getJSONBounded GETs endpoint and decodes at most maxThreadBodyBytes of JSON into out.
func getJSONBounded(ctx context.Context, endpoint, token, what string, out any) error {
	resp, err := doJSONRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%s failed with status %d", what, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxThreadBodyBytes)).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", what, err)
	}
	return nil
}

func fetchMastodonCompatibleThread(ctx context.Context, instanceBaseURL, accessToken, statusGETPath string, target ThreadTarget) ([]ThreadReply, error) {
	id := strings.TrimSpace(target.RemoteID)
	if id == "" && strings.TrimSpace(target.PublishedURL) == "" {
		return nil, fmt.Errorf("remote id or published url is required for thread")
	}
	if strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("access token is required for thread")
	}
	base := strings.TrimRight(strings.TrimSpace(instanceBaseURL), "/")
	path := strings.TrimSpace(statusGETPath)
	if path == "" {
		path = "/api/v1/statuses"
	}
	if id == "" {
		var err error
		if id, err = mastodonStatusIDFromPublishedURL(target.PublishedURL); err != nil {
			return nil, err
		}
	}

	var payload struct {
		Descendants []mastodonThreadStatus `json:"descendants"`
	}
	if err := getJSONBounded(ctx, base+path+"/"+url.PathEscape(id)+"/context", accessToken, "thread context", &payload); err != nil {
		return nil, err
	}

	out := make([]ThreadReply, 0, len(payload.Descendants))
	for _, st := range payload.Descendants {
		if len(out) >= maxThreadReplies {
			break
		}
		// Private mentions must never be shown to every team viewer.
		if strings.EqualFold(st.Visibility, "direct") {
			continue
		}
		remoteID := strings.TrimSpace(st.ID)
		if remoteID == "" {
			continue
		}
		publishedAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(st.CreatedAt))
		link := strings.TrimSpace(st.URL)
		if link == "" {
			link = strings.TrimSpace(st.URI)
		}
		avatar := strings.TrimSpace(st.Account.AvatarStatic)
		if avatar == "" {
			avatar = strings.TrimSpace(st.Account.Avatar)
		}
		out = append(out, ThreadReply{
			AuthorPost: AuthorPost{
				RemoteID:    remoteID,
				URL:         link,
				Content:     htmlToPlainText(st.Content),
				PublishedAt: publishedAt.UTC(),
			},
			Author: PostAuthor{
				Handle:      strings.TrimSpace(st.Account.Acct),
				DisplayName: strings.TrimSpace(st.Account.DisplayName),
				AvatarURL:   avatar,
				ProfileURL:  strings.TrimSpace(st.Account.URL),
			},
			ParentID: strings.TrimSpace(st.InReplyToID),
		})
	}
	return out, nil
}

func (p *MastodonProvider) GetThread(ctx context.Context, account domain.SocialAccount, auth PublishAuth, target ThreadTarget) ([]ThreadReply, error) {
	return fetchMastodonCompatibleThread(ctx, account.InstanceURL, auth.AccessToken, "/api/v1/statuses", target)
}

func (p *GenericStatusProvider) GetThread(ctx context.Context, account domain.SocialAccount, auth PublishAuth, target ThreadTarget) ([]ThreadReply, error) {
	return fetchMastodonCompatibleThread(ctx, account.InstanceURL, auth.AccessToken, p.postPath, target)
}
