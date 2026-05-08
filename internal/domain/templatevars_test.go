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
