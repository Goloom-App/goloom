// Package pagefetch reads public web pages with SSRF and resource limits.
package pagefetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/htmltext"
	"git.f4mily.net/goloom/internal/netguard"
)

const maxBodyBytes = 2 << 20

// FetchBody guards the initial URL, redirects, and the actual dial.
func FetchBody(ctx context.Context, rawURL string, policy netguard.Policy, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("source_url is required")
	}
	if err := policy.CheckURL(ctx, rawURL); err != nil {
		return "", err
	}
	var lastErr error
	for _, candidate := range append(GitHubRawCandidates(rawURL), rawURL) {
		if err := policy.CheckURL(ctx, candidate); err != nil {
			lastErr = err
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "goloom-page-fetch/1.0")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")
		resp, err := policy.HTTPClient(timeout).Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
		_ = resp.Body.Close()
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = http.DetectContentType(body)
		}
		switch {
		case readErr != nil:
			lastErr = readErr
		case resp.StatusCode < 200 || resp.StatusCode >= 300:
			lastErr = fmt.Errorf("url fetch failed: HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		case !readableContentType(contentType):
			lastErr = errors.New("url fetch returned non-text content")
		case len(body) > maxBodyBytes:
			lastErr = errors.New("url fetch body exceeds size limit")
		default:
			return string(body), nil
		}
	}
	return "", lastErr
}

func readableContentType(raw string) bool {
	if raw == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return false
	}
	return strings.HasPrefix(mediaType, "text/") || mediaType == "application/xhtml+xml" ||
		mediaType == "application/json" || mediaType == "application/xml" ||
		strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
}

func FetchText(ctx context.Context, rawURL string, policy netguard.Policy, timeout time.Duration) (string, error) {
	body, err := FetchBody(ctx, rawURL, policy, timeout)
	if err != nil {
		return "", err
	}
	text := htmltext.ExtractReadableText(body)
	if text == "" {
		return "", errors.New("source_url_empty")
	}
	return text, nil
}
