package rss

import (
	"context"
	"fmt"
	"strings"
	"time"

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
