package mcp

import (
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// ParseWeekday converts a weekday name string to time.Weekday int.
// Supports "monday", "tuesday", etc. and "next_free_tuesday" format.
// Returns -1 if no specific weekday is requested (any day).
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

// FindNextFreeSlot finds the next date that has no scheduled posts.
// It searches from 'after' to 'before', optionally filtering by weekday.
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

// TruncateString truncates a string to maxLen characters, adding "..." if truncated.
func TruncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
