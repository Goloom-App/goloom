package scheduling

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RecurrenceWeekly                    = "weekly"
	RecurrenceMonthlyDOM                = "monthly_dom"
	RecurrenceMonthlyAnchorOffset       = "monthly_anchor_offset"
	RecurrenceMonthlyOrdinalWeekday     = "monthly_ordinal_weekday"
)

// RecurrenceRule is stored JSON in post_templates.recurrence_json.
type RecurrenceRule struct {
	Kind string `json:"kind"`

	Weekdays []int `json:"weekdays,omitempty"`
	Hour     int   `json:"hour"`
	Minute   int   `json:"minute"`
	Timezone string `json:"timezone"`

	DayOfMonth int `json:"day_of_month,omitempty"`

	AnchorDay  int `json:"anchor_day,omitempty"`
	OffsetDays int `json:"offset_days,omitempty"`

	Ordinal        int   `json:"ordinal,omitempty"`
	Ordinals       []int `json:"ordinals,omitempty"`
	OrdinalWeekday int   `json:"ordinal_weekday,omitempty"`
}

// ParseRecurrenceJSON validates and parses recurrence_json from the database.
func ParseRecurrenceJSON(raw string) (*RecurrenceRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("recurrence_json is required")
	}
	var r RecurrenceRule
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, fmt.Errorf("recurrence_json: %w", err)
	}
	if err := ValidateRecurrenceRule(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ValidateRecurrenceRule checks ranges for a parsed rule.
func ValidateRecurrenceRule(r *RecurrenceRule) error {
	if r == nil {
		return errors.New("rule is nil")
	}
	switch strings.TrimSpace(r.Kind) {
	case RecurrenceWeekly, RecurrenceMonthlyDOM, RecurrenceMonthlyAnchorOffset, RecurrenceMonthlyOrdinalWeekday:
	default:
		return fmt.Errorf("unsupported recurrence kind %q", strings.TrimSpace(r.Kind))
	}
	if r.Hour < 0 || r.Hour > 23 || r.Minute < 0 || r.Minute > 59 {
		return errors.New("hour/minute out of range")
	}
	tz := strings.TrimSpace(r.Timezone)
	if tz == "" {
		return errors.New("timezone is required")
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return fmt.Errorf("timezone: %w", err)
	}
	switch strings.TrimSpace(r.Kind) {
	case RecurrenceWeekly:
		if len(r.Weekdays) == 0 {
			return errors.New("weekly recurrence requires weekdays")
		}
		for _, wd := range r.Weekdays {
			if wd < 0 || wd > 6 {
				return errors.New("weekday must be 0–6 (Sunday–Saturday)")
			}
		}
	case RecurrenceMonthlyDOM:
		if r.DayOfMonth < 1 || r.DayOfMonth > 31 {
			return errors.New("day_of_month must be 1–31")
		}
	case RecurrenceMonthlyAnchorOffset:
		if r.AnchorDay < 1 || r.AnchorDay > 31 {
			return errors.New("anchor_day must be 1–31")
		}
	case RecurrenceMonthlyOrdinalWeekday:
		ordinals := effectiveOrdinals(r)
		if len(ordinals) == 0 {
			return errors.New("ordinals requires at least one occurrence (1–5 or -1 for last)")
		}
		for _, ord := range ordinals {
			if ord < -1 || ord == 0 || ord > 5 {
				return errors.New("each ordinal must be -1 (last), or 1–5")
			}
		}
		if r.OrdinalWeekday < 0 || r.OrdinalWeekday > 6 {
			return errors.New("ordinal_weekday must be 0–6 (Sunday–Saturday)")
		}
	}
	return nil
}

func effectiveOrdinals(r *RecurrenceRule) []int {
	if len(r.Ordinals) > 0 {
		out := make([]int, 0, len(r.Ordinals))
		seen := make(map[int]struct{}, len(r.Ordinals))
		for _, ord := range r.Ordinals {
			if ord < -1 || ord == 0 || ord > 5 {
				continue
			}
			if _, ok := seen[ord]; ok {
				continue
			}
			seen[ord] = struct{}{}
			out = append(out, ord)
		}
		return out
	}
	if r.Ordinal != 0 {
		return []int{r.Ordinal}
	}
	return nil
}

func loadLocation(tz string) *time.Location {
	loc, err := time.LoadLocation(strings.TrimSpace(tz))
	if err != nil {
		return time.UTC
	}
	return loc
}

// NextOccurrence returns the next scheduled instant strictly after `after` (interpreted in UTC).
func NextOccurrence(rule *RecurrenceRule, after time.Time) (time.Time, error) {
	if rule == nil {
		return time.Time{}, errors.New("rule is nil")
	}
	if err := ValidateRecurrenceRule(rule); err != nil {
		return time.Time{}, err
	}
	loc := loadLocation(rule.Timezone)
	t := after.UTC()
	switch strings.TrimSpace(rule.Kind) {
	case RecurrenceWeekly:
		return nextWeekly(rule, t, loc), nil
	case RecurrenceMonthlyDOM:
		return nextMonthlyDOM(rule, t, loc), nil
	case RecurrenceMonthlyAnchorOffset:
		return nextMonthlyAnchorOffset(rule, t, loc), nil
	case RecurrenceMonthlyOrdinalWeekday:
		return nextMonthlyOrdinalWeekday(rule, t, loc), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported kind %q", rule.Kind)
	}
}

func nextWeekly(rule *RecurrenceRule, after time.Time, loc *time.Location) time.Time {
	ref := after.In(loc)
	dayStart := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, loc)
	for d := 0; d < 800; d++ {
		cur := dayStart.AddDate(0, 0, d)
		wd := int(cur.Weekday())
		if !intSliceContains(rule.Weekdays, wd) {
			continue
		}
		instant := time.Date(cur.Year(), cur.Month(), cur.Day(), rule.Hour, rule.Minute, 0, 0, loc)
		if instant.After(ref) {
			return instant.UTC()
		}
	}
	return after.AddDate(2, 0, 0).UTC()
}

func nextMonthlyDOM(rule *RecurrenceRule, after time.Time, loc *time.Location) time.Time {
	ref := after.In(loc)
	y, mo := ref.Year(), ref.Month()
	for i := 0; i < 120; i++ {
		candMonth := time.Date(y, mo, 1, 0, 0, 0, 0, loc).AddDate(0, i, 0)
		yy, mm := candMonth.Year(), candMonth.Month()
		dom := rule.DayOfMonth
		maxd := daysInMonth(yy, int(mm))
		if dom > maxd {
			dom = maxd
		}
		instant := time.Date(yy, mm, dom, rule.Hour, rule.Minute, 0, 0, loc)
		if instant.After(ref) {
			return instant.UTC()
		}
	}
	return after.AddDate(10, 0, 0).UTC()
}

func nextMonthlyAnchorOffset(rule *RecurrenceRule, after time.Time, loc *time.Location) time.Time {
	ref := after.In(loc)
	y, mo := ref.Year(), ref.Month()
	for i := 0; i < 120; i++ {
		candMonth := time.Date(y, mo, 1, 0, 0, 0, 0, loc).AddDate(0, i, 0)
		yy, mm := candMonth.Year(), candMonth.Month()
		maxd := daysInMonth(yy, int(mm))
		raw := rule.AnchorDay + rule.OffsetDays
		day := raw
		if day < 1 {
			day = 1
		}
		if day > maxd {
			day = maxd
		}
		instant := time.Date(yy, mm, day, rule.Hour, rule.Minute, 0, 0, loc)
		if instant.After(ref) {
			return instant.UTC()
		}
	}
	return after.AddDate(10, 0, 0).UTC()
}

func nextMonthlyOrdinalWeekday(rule *RecurrenceRule, after time.Time, loc *time.Location) time.Time {
	ref := after.In(loc)
	y, mo := ref.Year(), ref.Month()
	ordinals := effectiveOrdinals(rule)
	for i := 0; i < 120; i++ {
		candMonth := time.Date(y, mo, 1, 0, 0, 0, 0, loc).AddDate(0, i, 0)
		yy, mm := candMonth.Year(), candMonth.Month()

		var earliest time.Time
		found := false
		for _, ord := range ordinals {
			day, ok := ordinalWeekdayDay(yy, int(mm), ord, rule.OrdinalWeekday, loc)
			if !ok {
				continue
			}
			instant := time.Date(yy, mm, day, rule.Hour, rule.Minute, 0, 0, loc)
			if instant.After(ref) && (!found || instant.Before(earliest)) {
				earliest = instant
				found = true
			}
		}
		if found {
			return earliest.UTC()
		}
	}
	return after.AddDate(10, 0, 0).UTC()
}

func ordinalWeekdayDay(year, month, ordinal, weekday int, loc *time.Location) (int, bool) {
	maxd := daysInMonth(year, month)
	if ordinal == -1 {
		day := maxd
		for day > 0 {
			t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc)
			if int(t.Weekday()) == weekday {
				return day, true
			}
			day--
		}
		return 0, false
	}
	firstDay := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	offset := (weekday - int(firstDay.Weekday()) + 7) % 7
	day := 1 + offset + (ordinal-1)*7
	if day < 1 || day > maxd {
		return 0, false
	}
	return day, true
}

func daysInMonth(year, month int) int {
	t := time.Date(year, time.Month(month)+1, 0, 12, 0, 0, 0, time.UTC)
	return t.Day()
}

func intSliceContains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
