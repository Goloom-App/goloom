package agenttools

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// ParseWeekday converts a weekday name string to a time.Weekday int. Supports
// "monday", "tuesday", etc. and the "next_free_tuesday" form. Returns -1 when no
// specific weekday is requested (any day).
func ParseWeekday(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "next_free_")

	switch s {
	case "monday":
		return int(time.Monday)
	case "tuesday":
		return int(time.Tuesday)
	case "wednesday":
		return int(time.Wednesday)
	case "thursday":
		return int(time.Thursday)
	case "friday":
		return int(time.Friday)
	case "saturday":
		return int(time.Saturday)
	case "sunday":
		return int(time.Sunday)
	default:
		return -1
	}
}

// FindNextFreeSlot finds the next date that has no scheduled posts. It searches
// from 'after' to 'before', optionally filtering by weekday.
func FindNextFreeSlot(posts []domain.ScheduledPost, after, before time.Time, targetWeekday int) (time.Time, bool) {
	for d := after; d.Before(before); d = d.AddDate(0, 0, 1) {
		if targetWeekday >= 0 && d.Weekday() != time.Weekday(targetWeekday) {
			continue
		}
		hasPost := false
		for _, p := range posts {
			if p.ScheduledAt.Year() == d.Year() &&
				p.ScheduledAt.YearDay() == d.YearDay() &&
				p.Status != domain.PostStatusCancelled &&
				p.Status != domain.PostStatusFailed {
				hasPost = true
				break
			}
		}
		if !hasPost {
			return d, true
		}
	}
	return time.Time{}, false
}

// TruncateString truncates a string to maxLen characters, adding "..." if cut.
func TruncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// validateFeedURL rejects empty or non-http(s) RSS feed URLs before they are
// persisted, so a feed automation can never be created with an unusable source.
func validateFeedURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("feed_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("feed_url is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("feed_url must be an http(s) URL")
	}
	if u.Host == "" {
		return fmt.Errorf("feed_url must include a host")
	}
	return nil
}
