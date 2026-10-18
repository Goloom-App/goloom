package rss

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/netguard"
	"github.com/mmcdole/gofeed"
)

// Item is a normalized RSS/Atom feed entry.
type Item struct {
	GUID        string
	Link        string
	Title       string
	Content     string
	PublishedAt time.Time
}

type Parser struct {
	client *gofeed.Parser
}

func NewParser() *Parser {
	return &Parser{client: gofeed.NewParser()}
}

func (p *Parser) Fetch(ctx context.Context, feedURL string) ([]Item, error) {
	feedURL = strings.TrimSpace(feedURL)
	if feedURL == "" {
		return nil, fmt.Errorf("feed url is required")
	}
	parsed, err := p.client.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(parsed.Items))
	for _, entry := range parsed.Items {
		items = append(items, mapItem(entry))
	}
	return items, nil
}

// FetchWithPolicy parses a feed after fetching it through a guarded HTTP client.
// Caller owns timeout and policy; body is bounded to avoid untrusted feed growth.
func (p *Parser) FetchWithPolicy(ctx context.Context, feedURL string, policy netguard.Policy, timeout time.Duration) ([]Item, error) {
	feedURL = strings.TrimSpace(feedURL)
	if feedURL == "" {
		return nil, fmt.Errorf("feed url is required")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := policy.CheckURL(ctx, feedURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "goloom-rss-preview/1.0")
	resp, err := policy.HTTPClient(timeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("feed fetch failed: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 2<<20 {
		return nil, fmt.Errorf("feed body exceeds size limit")
	}
	parsed, err := p.client.ParseString(string(body))
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(parsed.Items))
	for _, entry := range parsed.Items {
		items = append(items, mapItem(entry))
	}
	return items, nil
}

func mapItem(entry *gofeed.Item) Item {
	content := ""
	if entry.Content != "" {
		content = entry.Content
	} else if entry.Description != "" {
		content = entry.Description
	}
	published := time.Now().UTC()
	if entry.PublishedParsed != nil {
		published = entry.PublishedParsed.UTC()
	} else if entry.UpdatedParsed != nil {
		published = entry.UpdatedParsed.UTC()
	}
	guid := strings.TrimSpace(entry.GUID)
	if guid == "" {
		guid = strings.TrimSpace(entry.Link)
	}
	return Item{
		GUID:        guid,
		Link:        strings.TrimSpace(entry.Link),
		Title:       strings.TrimSpace(entry.Title),
		Content:     content,
		PublishedAt: published,
	}
}

func ItemKey(item Item) string {
	if strings.TrimSpace(item.GUID) != "" {
		return strings.TrimSpace(item.GUID)
	}
	return strings.TrimSpace(item.Link)
}
