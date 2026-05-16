package api

import (
	"strings"

	"git.f4mily.net/goloom/internal/domain"
)

func decodeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&#34;", "\"")
	return s
}

func decodePostEngagementTitles(posts []domain.PostEngagementSummary) {
	for i := range posts {
		posts[i].Title = decodeHTMLEntities(posts[i].Title)
	}
}

func decodePostAnalyticsListTitles(items []domain.PostAnalyticsListRow) {
	for i := range items {
		items[i].Title = decodeHTMLEntities(items[i].Title)
	}
}
