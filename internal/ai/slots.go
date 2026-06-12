package ai

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

var weekdayNamesEN = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
var weekdayNamesDE = []string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}

// nowFunc is swapped in tests.
var nowFunc = func() time.Time { return time.Now().UTC() }

// formatScheduleLabel renders a human-readable schedule label for prompts (UTC).
func formatScheduleLabel(isoValue, language string) string {
	raw := strings.TrimSpace(isoValue)
	if raw == "" {
		return ""
	}
	dt, err := parseISOTime(raw)
	if err != nil {
		return raw
	}
	dt = dt.UTC()
	weekdayIndex := int(dt.Weekday())
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "de") {
		return fmt.Sprintf("%s, %02d.%02d.%d, %02d:%02d UTC", weekdayNamesDE[weekdayIndex], dt.Day(), int(dt.Month()), dt.Year(), dt.Hour(), dt.Minute())
	}
	return fmt.Sprintf("%s, %s, %s UTC", weekdayNamesEN[weekdayIndex], dt.Format("2006-01-02"), dt.Format("15:04"))
}

func parseISOTime(raw string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable time %q", raw)
}

func formatDatetime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05Z07:00")
}

// resolveScheduledAt picks a publication slot for generated posts.
func resolveScheduledAt(params params, campaignFormat *domain.CampaignFormat, context domain.AIContext) *time.Time {
	preferred := preferredPostingTime(context, campaignFormat)
	if targetDate := params.str("target_date"); targetDate != "" {
		if day, err := time.Parse("2006-01-02", strings.TrimSpace(targetDate)); err == nil {
			at := time.Date(day.Year(), day.Month(), day.Day(), preferred.hour, preferred.minute, 0, 0, time.UTC)
			return &at
		}
	}

	now := nowFunc()
	if campaignFormat == nil || campaignFormat.Weekday == nil {
		day := now.AddDate(0, 0, 1)
		at := time.Date(day.Year(), day.Month(), day.Day(), preferred.hour, preferred.minute, 0, 0, time.UTC)
		return &at
	}

	targetWeekday := *campaignFormat.Weekday
	occupied := occupiedCampaignDates(context, targetWeekday)
	for offset := 0; offset < 366; offset++ {
		candidate := now.AddDate(0, 0, offset)
		if int(candidate.Weekday()) != targetWeekday {
			continue
		}
		slot := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), preferred.hour, preferred.minute, 0, 0, time.UTC)
		if !slot.After(now) {
			continue
		}
		if occupied[slot.Format("2006-01-02")] {
			continue
		}
		return &slot
	}
	return nil
}

// NextCampaignSlot returns the next free publication slot for a campaign
// format, honoring its weekday, the team's scheduling preferences, and
// already scheduled posts.
func NextCampaignSlot(context domain.AIContext, format *domain.CampaignFormat) *time.Time {
	return resolveScheduledAt(params{}, format, context)
}

type clockTime struct {
	hour   int
	minute int
}

func preferredPostingTime(context domain.AIContext, campaignFormat *domain.CampaignFormat) clockTime {
	if hour, ok := bestEngagementHour(context.EngagementHours); ok {
		return clockTime{hour: hour}
	}

	prefs := context.Team.SchedulingPrefs
	if campaignFormat != nil && campaignFormat.Weekday != nil {
		for _, window := range prefs.PostingWindows {
			if window.Weekday == *campaignFormat.Weekday {
				if parsed, ok := parseClock(window.Start); ok {
					return parsed
				}
			}
		}
	}
	for _, slot := range prefs.DefaultTimeslots {
		if parsed, ok := parseClock(slot); ok {
			return parsed
		}
	}
	return clockTime{hour: 9}
}

func occupiedCampaignDates(context domain.AIContext, weekday int) map[string]bool {
	occupied := map[string]bool{}
	for _, post := range context.UpcomingPosts {
		at := post.ScheduledAt.UTC()
		if at.IsZero() {
			continue
		}
		if int(at.Weekday()) == weekday {
			occupied[at.Format("2006-01-02")] = true
		}
	}
	return occupied
}

func bestEngagementHour(buckets []domain.EngagementHourBucket) (int, bool) {
	bestHour := -1
	bestScore := int64(-1)
	for _, bucket := range buckets {
		if bucket.HourUTC < 0 || bucket.HourUTC > 23 {
			continue
		}
		if bucket.Score > bestScore {
			bestScore = bucket.Score
			bestHour = bucket.HourUTC
		}
	}
	return bestHour, bestHour >= 0
}

func parseClock(raw string) (clockTime, bool) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) < 2 {
		return clockTime{}, false
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return clockTime{}, false
	}
	return clockTime{hour: hour, minute: minute}, true
}
