package domain

import (
	"strings"
	"testing"
)

func TestDefaultTeamSchedulingPreferences(t *testing.T) {
	t.Parallel()
	p := DefaultTeamSchedulingPreferences()
	if p.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC", p.Timezone)
	}
	if p.PostingWindows != nil {
		t.Errorf("PostingWindows should be nil, got %v", p.PostingWindows)
	}
	if p.DefaultTimeslots != nil {
		t.Errorf("DefaultTimeslots should be nil, got %v", p.DefaultTimeslots)
	}
}

func TestNormalizeTeamSchedulingPrefs(t *testing.T) {
	t.Parallel()

	t.Run("empty timezone defaults to UTC", func(t *testing.T) {
		p := NormalizeTeamSchedulingPrefs(TeamSchedulingPreferences{Timezone: "  "})
		if p.Timezone != "UTC" {
			t.Errorf("Timezone = %q, want UTC", p.Timezone)
		}
	})

	t.Run("timezone is trimmed", func(t *testing.T) {
		p := NormalizeTeamSchedulingPrefs(TeamSchedulingPreferences{Timezone: "  Europe/Berlin  "})
		if p.Timezone != "Europe/Berlin" {
			t.Errorf("Timezone = %q", p.Timezone)
		}
	})

	t.Run("posting windows are preserved", func(t *testing.T) {
		in := TeamSchedulingPreferences{
			Timezone: "UTC",
			PostingWindows: []TeamPostingWindow{
				{Weekday: 1, Start: " 09:00 ", End: " 17:00 "},
			},
		}
		p := NormalizeTeamSchedulingPrefs(in)
		if len(p.PostingWindows) != 1 {
			t.Fatalf("expected 1 window, got %d", len(p.PostingWindows))
		}
		w := p.PostingWindows[0]
		if w.Weekday != 1 || w.Start != "09:00" || w.End != "17:00" {
			t.Errorf("window = %+v", w)
		}
	})

	t.Run("blank timeslots are dropped", func(t *testing.T) {
		in := TeamSchedulingPreferences{
			Timezone:         "UTC",
			DefaultTimeslots: []string{"10:00", "  ", "14:00", ""},
		}
		p := NormalizeTeamSchedulingPrefs(in)
		if len(p.DefaultTimeslots) != 2 {
			t.Errorf("want 2 slots, got %v", p.DefaultTimeslots)
		}
	})

	t.Run("timeslots are trimmed", func(t *testing.T) {
		in := TeamSchedulingPreferences{
			Timezone:         "UTC",
			DefaultTimeslots: []string{" 08:30 "},
		}
		p := NormalizeTeamSchedulingPrefs(in)
		if len(p.DefaultTimeslots) != 1 || p.DefaultTimeslots[0] != "08:30" {
			t.Errorf("slots = %v", p.DefaultTimeslots)
		}
	})
}

func TestParseTeamSchedulingPrefsJSON(t *testing.T) {
	t.Parallel()

	t.Run("empty string returns default", func(t *testing.T) {
		p, err := ParseTeamSchedulingPrefsJSON("")
		if err != nil {
			t.Fatal(err)
		}
		if p.Timezone != "UTC" {
			t.Errorf("Timezone = %q", p.Timezone)
		}
	})

	t.Run("empty object returns default", func(t *testing.T) {
		p, err := ParseTeamSchedulingPrefsJSON("{}")
		if err != nil {
			t.Fatal(err)
		}
		if p.Timezone != "UTC" {
			t.Errorf("Timezone = %q", p.Timezone)
		}
	})

	t.Run("valid JSON is parsed and normalized", func(t *testing.T) {
		raw := `{"timezone":"  America/New_York  ","default_timeslots":["10:00",""]}`
		p, err := ParseTeamSchedulingPrefsJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
		if p.Timezone != "America/New_York" {
			t.Errorf("Timezone = %q", p.Timezone)
		}
		if len(p.DefaultTimeslots) != 1 || p.DefaultTimeslots[0] != "10:00" {
			t.Errorf("slots = %v", p.DefaultTimeslots)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := ParseTeamSchedulingPrefsJSON("{not json}")
		if err == nil {
			t.Fatal("expected parse error")
		}
	})

	t.Run("whitespace-only input returns default", func(t *testing.T) {
		p, err := ParseTeamSchedulingPrefsJSON("   ")
		if err != nil {
			t.Fatal(err)
		}
		if p.Timezone != "UTC" {
			t.Errorf("Timezone = %q", p.Timezone)
		}
	})
}

func TestEncodeTeamSchedulingPrefsJSON(t *testing.T) {
	t.Parallel()

	t.Run("round-trips timezone", func(t *testing.T) {
		in := TeamSchedulingPreferences{
			Timezone:         "Europe/Berlin",
			DefaultTimeslots: []string{"09:00", "15:00"},
		}
		raw, err := EncodeTeamSchedulingPrefsJSON(in)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(raw, "Europe/Berlin") {
			t.Errorf("encoded = %q", raw)
		}
		if !strings.Contains(raw, "09:00") {
			t.Errorf("encoded = %q", raw)
		}
	})

	t.Run("empty timezone is defaulted to UTC before encoding", func(t *testing.T) {
		in := TeamSchedulingPreferences{}
		raw, err := EncodeTeamSchedulingPrefsJSON(in)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(raw, "UTC") {
			t.Errorf("encoded = %q", raw)
		}
	})

	t.Run("encode-then-parse is idempotent", func(t *testing.T) {
		original := TeamSchedulingPreferences{
			Timezone:         "America/New_York",
			PostingWindows:   []TeamPostingWindow{{Weekday: 2, Start: "10:00", End: "12:00"}},
			DefaultTimeslots: []string{"10:00"},
		}
		raw, err := EncodeTeamSchedulingPrefsJSON(original)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseTeamSchedulingPrefsJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got.Timezone != original.Timezone {
			t.Errorf("Timezone round-trip: got %q, want %q", got.Timezone, original.Timezone)
		}
		if len(got.PostingWindows) != 1 || got.PostingWindows[0].Start != "10:00" {
			t.Errorf("PostingWindows round-trip: %+v", got.PostingWindows)
		}
	})
}
