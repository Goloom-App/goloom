package domain

import (
	"encoding/json"
	"strings"
)

// TeamSchedulingPreferences holds team-level smart scheduling defaults (JSON in teams.scheduling_prefs).
type TeamSchedulingPreferences struct {
	Timezone         string              `json:"timezone"`
	PostingWindows   []TeamPostingWindow `json:"posting_windows"`
	DefaultTimeslots []string            `json:"default_timeslots"`
}

type TeamPostingWindow struct {
	Weekday int    `json:"weekday"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

// DefaultTeamSchedulingPreferences returns a safe empty document.
func DefaultTeamSchedulingPreferences() TeamSchedulingPreferences {
	return TeamSchedulingPreferences{
		Timezone:         "UTC",
		PostingWindows:   nil,
		DefaultTimeslots: nil,
	}
}

// NormalizeTeamSchedulingPrefs trims strings and defaults timezone.
func NormalizeTeamSchedulingPrefs(p TeamSchedulingPreferences) TeamSchedulingPreferences {
	tz := strings.TrimSpace(p.Timezone)
	if tz == "" {
		tz = "UTC"
	}
	var windows []TeamPostingWindow
	for _, w := range p.PostingWindows {
		windows = append(windows, TeamPostingWindow{
			Weekday: w.Weekday,
			Start:   strings.TrimSpace(w.Start),
			End:     strings.TrimSpace(w.End),
		})
	}
	var slots []string
	for _, s := range p.DefaultTimeslots {
		s = strings.TrimSpace(s)
		if s != "" {
			slots = append(slots, s)
		}
	}
	return TeamSchedulingPreferences{
		Timezone:         tz,
		PostingWindows:   windows,
		DefaultTimeslots: slots,
	}
}

// ParseTeamSchedulingPrefsJSON unmarshals team scheduling_prefs column text.
func ParseTeamSchedulingPrefsJSON(raw string) (TeamSchedulingPreferences, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return DefaultTeamSchedulingPreferences(), nil
	}
	var p TeamSchedulingPreferences
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return TeamSchedulingPreferences{}, err
	}
	return NormalizeTeamSchedulingPrefs(p), nil
}

// EncodeTeamSchedulingPrefsJSON marshals preferences for storage.
func EncodeTeamSchedulingPrefsJSON(p TeamSchedulingPreferences) (string, error) {
	p = NormalizeTeamSchedulingPrefs(p)
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
