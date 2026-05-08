package domain

import (
	"strconv"
	"strings"
	"time"
)

// ExpandDynamicVariables replaces {year}, {month}, {day}, and {counter} in content at publish time.
// Month and day are zero-padded to width 2. If counter is nil, {counter} becomes an empty string.
func ExpandDynamicVariables(content string, publishedAt time.Time, counter *int) string {
	if content == "" {
		return ""
	}
	t := publishedAt.UTC()
	repl := []struct {
		old string
		new string
	}{
		{"{year}", strconv.Itoa(t.Year())},
		{"{month}", zeroPad2(int(t.Month()))},
		{"{day}", zeroPad2(t.Day())},
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

func zeroPad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
