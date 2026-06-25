package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/htmltext"
)

func fetchURLBody(ctx context.Context, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("source_url is required")
	}

	// GitHub web pages render their content client-side, so fetch the raw source
	// (README/blob) instead. Candidates are tried in order; the original URL is
	// the final fallback, so non-GitHub URLs and unrewritable pages behave as
	// before.
	urls := append(githubRawCandidates(rawURL), rawURL)
	var lastErr error
	for _, u := range urls {
		body, err := httpGetBody(ctx, u)
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func httpGetBody(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "goloom-page-fetch/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("url fetch failed: HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return string(body), nil
}

func fetchURLText(ctx context.Context, rawURL string) (string, error) {
	body, err := fetchURLBody(ctx, rawURL)
	if err != nil {
		return "", err
	}
	return extractReadableTextFromHTML(body), nil
}

func enrichParamsFromWebPage(ctx context.Context, params map[string]any, pageURL string) error {
	if strings.TrimSpace(stringParam(params["source_content"])) != "" {
		return nil
	}

	body, err := fetchURLBody(ctx, pageURL)
	if err != nil {
		return err
	}

	content := htmltext.ExtractReadableText(body)
	if strings.TrimSpace(content) == "" {
		return errors.New("source_url_empty")
	}

	params["source_content"] = content
	if title := htmltext.ExtractPageTitle(body); title != "" {
		params["page_title"] = title
	}
	return nil
}
