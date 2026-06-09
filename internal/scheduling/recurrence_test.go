package scheduling

import (
	"testing"
	"time"
)

func TestNextOccurrence_weekly(t *testing.T) {
	t.Parallel()
	rule := &RecurrenceRule{
		Kind:     RecurrenceWeekly,
		Weekdays: []int{1}, // Monday
		Hour:     9,
		Minute:   30,
		Timezone: "UTC",
	}
	// 2025-05-05 is Monday UTC
	start := time.Date(2025, 5, 5, 10, 0, 0, 0, time.UTC)
	next, err := NextOccurrence(rule, start)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2025, 5, 12, 9, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
}

func TestNextOccurrence_monthlyDOM(t *testing.T) {
	t.Parallel()
	rule := &RecurrenceRule{
		Kind:       RecurrenceMonthlyDOM,
		DayOfMonth: 15,
		Hour:       14,
		Minute:     0,
		Timezone:   "UTC",
	}
	start := time.Date(2025, 5, 10, 0, 0, 0, 0, time.UTC)
	next, err := NextOccurrence(rule, start)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2025, 5, 15, 14, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
}

func TestNextOccurrence_ordinalWeekday(t *testing.T) {
	t.Parallel()
	rule := &RecurrenceRule{
		Kind:           RecurrenceMonthlyOrdinalWeekday,
		Ordinal:        2,
		OrdinalWeekday: 1,
		Hour:           9,
		Minute:         0,
		Timezone:       "UTC",
	}
	start := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	next, err := NextOccurrence(rule, start)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2025, 5, 12, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
}

func TestNextOccurrence_ordinalLastMonday(t *testing.T) {
	t.Parallel()
	rule := &RecurrenceRule{
		Kind:           RecurrenceMonthlyOrdinalWeekday,
		Ordinal:        -1,
		OrdinalWeekday: 1,
		Hour:           9,
		Minute:         0,
		Timezone:       "UTC",
	}
	start := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	next, err := NextOccurrence(rule, start)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2025, 5, 26, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
}

func TestNextOccurrence_ordinalFifthFriday(t *testing.T) {
	t.Parallel()
	rule := &RecurrenceRule{
		Kind:           RecurrenceMonthlyOrdinalWeekday,
		Ordinal:        5,
		OrdinalWeekday: 5,
		Hour:           10,
		Minute:         0,
		Timezone:       "UTC",
	}
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := NextOccurrence(rule, start)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2025, 1, 31, 10, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
}

func TestNextOccurrence_ordinalMultipleFridays(t *testing.T) {
	t.Parallel()
	rule := &RecurrenceRule{
		Kind:           RecurrenceMonthlyOrdinalWeekday,
		Ordinals:       []int{1, 3},
		OrdinalWeekday: 5, // Friday
		Hour:           9,
		Minute:         0,
		Timezone:       "UTC",
	}
	start := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	next, err := NextOccurrence(rule, start)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2025, 6, 6, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}

	start = time.Date(2025, 6, 7, 0, 0, 0, 0, time.UTC)
	next, err = NextOccurrence(rule, start)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2025, 6, 20, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("after first Friday got %v want %v", next, want)
	}
}

func TestNextOccurrence_ordinalLegacySingleField(t *testing.T) {
	t.Parallel()
	rule := &RecurrenceRule{
		Kind:           RecurrenceMonthlyOrdinalWeekday,
		Ordinal:        2,
		OrdinalWeekday: 1,
		Hour:           9,
		Minute:         0,
		Timezone:       "UTC",
	}
	start := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	next, err := NextOccurrence(rule, start)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2025, 5, 12, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
}

func TestNextOccurrence_anchorOffsetThreeDaysBefore(t *testing.T) {
	t.Parallel()
	rule := &RecurrenceRule{
		Kind:       RecurrenceMonthlyAnchorOffset,
		AnchorDay:  15,
		OffsetDays: -3,
		Hour:       8,
		Minute:     0,
		Timezone:   "UTC",
	}
	start := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	next, err := NextOccurrence(rule, start)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2025, 5, 12, 8, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
}
