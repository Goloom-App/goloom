package domain

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	dayOffsetRe   = regexp.MustCompile(`\{day([+-]\d+)\}`)
	monthOffsetRe = regexp.MustCompile(`\{month([+-]\d+)\}`)
)

var weekdayNames = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// ExpandDynamicVariables replaces {year}, {month}, {day}, {counter}, and new
// variables ({day+N}, {month+N}, {weekday}, {weekday_name}) in content.
// Month and day are zero-padded to width 2. If counter is nil, {counter} becomes "".
func ExpandDynamicVariables(content string, publishedAt time.Time, counter *int) string {
	if content == "" {
		return ""
	}
	t := publishedAt.UTC()

	content = dayOffsetRe.ReplaceAllStringFunc(content, func(m string) string {
		offset := parseOffset(m)
		return zeroPad2(clampDay(t.Day() + offset))
	})

	content = monthOffsetRe.ReplaceAllStringFunc(content, func(m string) string {
		offset := parseOffset(m)
		return zeroPad2(clampMonth(int(t.Month()) + offset))
	})

	repl := []struct {
		old string
		new string
	}{
		{"{year}", strconv.Itoa(t.Year())},
		{"{month}", zeroPad2(int(t.Month()))},
		{"{day}", zeroPad2(t.Day())},
		{"{weekday}", strconv.Itoa(int(t.Weekday()))},
		{"{weekday_name}", weekdayNames[t.Weekday()]},
	}
	out := content
	for _, r := range repl {
		out = strings.ReplaceAll(out, r.old, r.new)
	}

	counterStr := ""
	if counter != nil {
		counterStr = strconv.Itoa(*counter)
	}
	out = strings.ReplaceAll(out, "{counter}", counterStr)
	return out
}

func parseOffset(s string) int {
	if len(s) < 4 {
		return 0
	}
	inner := s[1 : len(s)-1]
	signIdx := -1
	for i, c := range inner {
		if c == '+' || c == '-' {
			signIdx = i
			break
		}
	}
	if signIdx < 0 {
		return 0
	}
	n, err := strconv.Atoi(inner[signIdx:])
	if err != nil {
		return 0
	}
	return n
}

func clampDay(d int) int {
	if d < 1 {
		return 1
	}
	if d > 31 {
		return 31
	}
	return d
}

func clampMonth(m int) int {
	return int(math.Max(1, math.Min(12, float64(m))))
}

func zeroPad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
