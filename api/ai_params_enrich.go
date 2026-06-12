package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"git.f4mily.net/goloom/internal/htmltext"
	"git.f4mily.net/goloom/internal/rss"
)

func enrichAIJobParams(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	if len(bytesTrim(raw)) == 0 {
		return json.RawMessage(`{}`), nil
	}

	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return raw, nil
	}

	feedURL := strings.TrimSpace(stringParam(params["rss_feed_url"]))
	if feedURL != "" {
		if err := enrichParamsFromRSSFeed(ctx, params, feedURL); err != nil {
			return nil, err
		}
	}

	pageURL := strings.TrimSpace(stringParam(params["source_url"]))
	if pageURL == "" {
		pageURL = strings.TrimSpace(stringParam(params["sourceUrl"]))
	}
	if pageURL != "" {
		if err := enrichParamsFromWebPage(ctx, params, pageURL); err != nil {
			return nil, fmt.Errorf("source_url_fetch_failed: %w", err)
		}
	}

	out, err := json.Marshal(params)
	if err != nil {
		return raw, err
	}
	return out, nil
}

func enrichParamsFromRSSFeed(ctx context.Context, params map[string]any, feedURL string) error {
	parser := rss.NewParser()
	items, err := parser.Fetch(ctx, feedURL)
	if err != nil {
		return fmt.Errorf("rss_feed_fetch_failed: %w", err)
	}
	if len(items) == 0 {
		return fmt.Errorf("rss_feed_empty")
	}

	latest := items[0]
	title := strings.TrimSpace(latest.Title)
	link := strings.TrimSpace(latest.Link)
	content := htmltext.ExtractReadableText(strings.TrimSpace(latest.Content))
	if title == "" && link == "" && content == "" {
		return fmt.Errorf("rss_feed_latest_item_empty")
	}

	params["rss_article_title"] = title
	params["rss_article_link"] = link
	params["rss_article_content"] = content
	return nil
}

func stringParam(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func bytesTrim(raw json.RawMessage) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}
