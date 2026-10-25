package provider

import (
	"context"
	"encoding/json"
	"errors"
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
	// maxThreadBodyBytes bounds the context response read from the remote instance.
	maxThreadBodyBytes = 8 << 20
)

var (
	htmlBreakRe     = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlParagraphRe = regexp.MustCompile(`(?i)</p>\s*<p[^>]*>`)

	// threadRetryPolicy never retries: this is an interactive read, a 429 must fail fast.
	threadRetryPolicy = RetryPolicy{}

	errThreadBodyTooLarge = fmt.Errorf("thread context response exceeds %d bytes", maxThreadBodyBytes)
)

// htmlToPlainText converts status HTML to text, keeping line breaks (<br> -> \n, paragraph break -> blank line).
func htmlToPlainText(raw string) string {
	s := htmlBreakRe.ReplaceAllString(raw, "\n")
	s = htmlParagraphRe.ReplaceAllString(s, "\n\n")
	return stripHTMLContent(s)
}

// threadVisible is an allowlist: followers-only ("private") replies stay visible to team viewers,
// direct, empty, unknown or fork-specific values are dropped.
func threadVisible(visibility string) bool {
	switch strings.ToLower(strings.TrimSpace(visibility)) {
	case "public", "unlisted", "private":
		return true
	}
	return false
}

// httpURLOrEmpty returns raw only if it is an http(s) URL, otherwise "".
func httpURLOrEmpty(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return raw
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
	SpoilerText string                `json:"spoiler_text"`
	Sensitive   bool                  `json:"sensitive"`
	Account     mastodonThreadAccount `json:"account"`
}

// boundedReader returns errThreadBodyTooLarge (not a silent EOF) once limit bytes were consumed.
type boundedReader struct {
	r io.Reader
	n int64
}

func (b *boundedReader) Read(p []byte) (int, error) {
	if b.n <= 0 {
		// Distinguish "exactly at limit" from "more data": probe one byte.
		var one [1]byte
		if n, _ := b.r.Read(one[:]); n > 0 {
			return 0, errThreadBodyTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > b.n {
		p = p[:b.n]
	}
	n, err := b.r.Read(p)
	b.n -= int64(n)
	return n, err
}

func wrapThreadDecodeErr(err error) error {
	switch {
	case errors.Is(err, errThreadBodyTooLarge):
		return err
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return errors.New("thread context response truncated")
	}
	return fmt.Errorf("decode thread context: %w", err)
}

// decodeThreadContext stream-decodes {"descendants":[...]} and keeps at most maxThreadReplies visible
// replies, without reading the remainder of the body once the cap is hit.
func decodeThreadContext(r io.Reader) (ThreadResult, error) {
	dec := json.NewDecoder(&boundedReader{r: r, n: maxThreadBodyBytes})
	expectDelim := func(want json.Delim) error {
		tok, err := dec.Token()
		if err != nil {
			return wrapThreadDecodeErr(err)
		}
		if d, ok := tok.(json.Delim); !ok || d != want {
			return fmt.Errorf("decode thread context: unexpected token %v", tok)
		}
		return nil
	}
	if err := expectDelim('{'); err != nil {
		return ThreadResult{}, err
	}
	res := ThreadResult{Replies: []ThreadReply{}}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return ThreadResult{}, wrapThreadDecodeErr(err)
		}
		if key, _ := keyTok.(string); key != "descendants" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return ThreadResult{}, wrapThreadDecodeErr(err)
			}
			continue
		}
		if err := expectDelim('['); err != nil {
			return ThreadResult{}, err
		}
		for dec.More() {
			if len(res.Replies) >= maxThreadReplies {
				res.Truncated = true
				return res, nil
			}
			var st mastodonThreadStatus
			if err := dec.Decode(&st); err != nil {
				return ThreadResult{}, wrapThreadDecodeErr(err)
			}
			if reply, ok := mapThreadStatus(st); ok {
				res.Replies = append(res.Replies, reply)
			}
		}
		if err := expectDelim(']'); err != nil {
			return ThreadResult{}, err
		}
	}
	if err := expectDelim('}'); err != nil {
		return ThreadResult{}, err
	}
	return res, nil
}

func mapThreadStatus(st mastodonThreadStatus) (ThreadReply, bool) {
	// Private mentions must never be shown to every team viewer.
	if !threadVisible(st.Visibility) {
		return ThreadReply{}, false
	}
	remoteID := strings.TrimSpace(st.ID)
	if remoteID == "" {
		return ThreadReply{}, false
	}
	publishedAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(st.CreatedAt))
	link := httpURLOrEmpty(st.URL)
	if link == "" {
		link = httpURLOrEmpty(st.URI)
	}
	avatar := httpURLOrEmpty(st.Account.AvatarStatic)
	if avatar == "" {
		avatar = httpURLOrEmpty(st.Account.Avatar)
	}
	return ThreadReply{
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
			ProfileURL:  httpURLOrEmpty(st.Account.URL),
		},
		ParentID:       strings.TrimSpace(st.InReplyToID),
		ContentWarning: strings.TrimSpace(st.SpoilerText),
		Sensitive:      st.Sensitive,
	}, true
}

func fetchMastodonCompatibleThread(ctx context.Context, instanceBaseURL, accessToken, statusGETPath string, target ThreadTarget) (ThreadResult, error) {
	if strings.TrimSpace(accessToken) == "" {
		return ThreadResult{}, fmt.Errorf("access token is required for thread")
	}
	id := strings.TrimSpace(target.RemoteID)
	if id == "" {
		if strings.TrimSpace(target.PublishedURL) == "" {
			return ThreadResult{}, ErrThreadTargetUnresolvable
		}
		var err error
		if id, err = mastodonStatusIDFromPublishedURL(target.PublishedURL); err != nil {
			return ThreadResult{}, fmt.Errorf("%w: %v", ErrThreadTargetUnresolvable, err)
		}
	}
	base := strings.TrimRight(strings.TrimSpace(instanceBaseURL), "/")
	baseURL, err := url.Parse(base)
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return ThreadResult{}, errors.New("instance url must be http(s)")
	}
	if err := validateInstanceOutboundURL(ctx, baseURL); err != nil {
		return ThreadResult{}, err
	}
	path := strings.TrimSpace(statusGETPath)
	if path == "" {
		path = "/api/v1/statuses"
	}

	resp, err := doJSONRequestWithPolicy(ctx, threadRetryPolicy, http.MethodGet, base+path+"/"+url.PathEscape(id)+"/context", accessToken, nil)
	if err != nil {
		return ThreadResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return ThreadResult{}, fmt.Errorf("thread context failed with status %d", resp.StatusCode)
	}
	return decodeThreadContext(resp.Body)
}

func (p *MastodonProvider) GetThread(ctx context.Context, account domain.SocialAccount, auth PublishAuth, target ThreadTarget) (ThreadResult, error) {
	return fetchMastodonCompatibleThread(ctx, account.InstanceURL, auth.AccessToken, "/api/v1/statuses", target)
}

func (p *GenericStatusProvider) GetThread(ctx context.Context, account domain.SocialAccount, auth PublishAuth, target ThreadTarget) (ThreadResult, error) {
	return fetchMastodonCompatibleThread(ctx, account.InstanceURL, auth.AccessToken, p.postPath, target)
}
