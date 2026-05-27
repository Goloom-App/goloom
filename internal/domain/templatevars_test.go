package domain

import (
	"testing"
	"time"
)

func TestExpandDynamicVariables(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	c := 42
	got := ExpandDynamicVariables("Hello {year}-{month}-{day} #{counter}", ts, &c)
	want := "Hello 2026-03-07 #42"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	gotNil := ExpandDynamicVariables("{counter}", ts, nil)
	if gotNil != "" {
		t.Fatalf("counter nil: got %q", gotNil)
	}
}

func TestExpandDynamicVariables_weekday(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	got := ExpandDynamicVariables("wd={weekday} name={weekday_name}", ts, nil)
	want := "wd=6 name=Sat"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestExpandDynamicVariables_dayOffset(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	got := ExpandDynamicVariables("day+2={day+2} day-1={day-1} month+1={month+1}", ts, nil)
	want := "day+2=09 day-1=06 month+1=04"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
