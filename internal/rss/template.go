package rss

import (
	"html"
	"regexp"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

var tagRe = regexp.MustCompile(`<[^>]+>`)

// ItemFields holds RSS item data for template expansion.
type ItemFields struct {
	Title         string
	Link          string
	Summary       string
	FeedName      string
	PublishedAt   time.Time
	Counter       int
}

func StripHTML(value string) string {
	text := html.UnescapeString(tagRe.ReplaceAllString(value, " "))
	return strings.Join(strings.Fields(text), " ")
}

// ExpandContent renders an automation content template with RSS fields and shared date/counter variables.
func ExpandContent(template string, fields ItemFields) string {
	template = strings.TrimSpace(template)
	if template == "" {
		template = domain.DefaultRSSContentTemplate
	}
	summary := StripHTML(fields.Summary)
	published := fields.PublishedAt.UTC()
	repl := map[string]string{
		"{title}":          strings.TrimSpace(fields.Title),
		"{link}":           strings.TrimSpace(fields.Link),
		"{summary}":        summary,
		"{feed_name}":      strings.TrimSpace(fields.FeedName),
		"{published_date}": published.Format("2006-01-02"),
		"{published_time}": published.Format("15:04"),
	}
	out := template
	for old, val := range repl {
		out = strings.ReplaceAll(out, old, val)
	}
	counter := fields.Counter
	return domain.ExpandDynamicVariables(out, published, &counter, nil)
}
