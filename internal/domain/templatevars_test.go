package domain

import (
	"testing"
	"time"
)

func TestExpandDynamicVariables(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	c := 42
	got := ExpandDynamicVariables("Hello {year}-{month}-{day} #{counter}", ts, &c, nil, nil)
	want := "Hello 2026-03-07 #42"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	gotNil := ExpandDynamicVariables("{counter}", ts, nil, nil, nil)
	if gotNil != "" {
		t.Fatalf("counter nil: got %q", gotNil)
	}
}

func TestExpandDynamicVariables_weekday(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	got := ExpandDynamicVariables("wd={weekday} name={weekday_name}", ts, nil, nil, nil)
	want := "wd=6 name=Sat"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestExpandDynamicVariables_dayOffset(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	got := ExpandDynamicVariables("day+2={day+2} day-1={day-1} month+1={month+1}", ts, nil, nil, nil)
	want := "day+2=09 day-1=06 month+1=04"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestExpandDynamicVariables_mainEvent(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	main := time.Date(2026, 6, 5, 18, 0, 0, 0, time.UTC)
	got := ExpandDynamicVariables("main_day={main_day} main_month={main_month} main_weekday={main_weekday_name}", ts, nil, &main, nil)
	want := "main_day=05 main_month=06 main_weekday=Fri"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// Without main event, {main_*} become empty
	gotNil := ExpandDynamicVariables("x={main_day}y", ts, nil, nil, nil)
	if gotNil != "x=y" {
		t.Fatalf("main nil: got %q", gotNil)
	}
}

func TestExpandDynamicVariables_mainCounter(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	main := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	ann := 3
	mainCtr := 7
	got := ExpandDynamicVariables("ann={counter} main={main_counter}", ts, &ann, &main, &mainCtr)
	want := "ann=3 main=7"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	gotNil := ExpandDynamicVariables("{main_counter}", ts, nil, nil, nil)
	if gotNil != "" {
		t.Fatalf("main_counter nil: got %q", gotNil)
	}
}
