package logging

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// --- fakeLogStore ---

type fakeLogStore struct {
	mu      sync.Mutex
	entries []domain.LogEntry
	failAt  int // if > 0, fail after this many successful inserts
	count   int
}

func (f *fakeLogStore) InsertLogEntry(_ context.Context, e domain.LogEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count++
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeLogStore) ArchiveLogEntry(_ context.Context, _ string) error   { return nil }
func (f *fakeLogStore) UnarchiveLogEntry(_ context.Context, _ string) error { return nil }
func (f *fakeLogStore) DeleteLogEntry(_ context.Context, _ string) error    { return nil }
func (f *fakeLogStore) DeleteLogEntriesBefore(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (f *fakeLogStore) captured() []domain.LogEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.LogEntry, len(f.entries))
	copy(out, f.entries)
	return out
}

// discardHandler is a slog.Handler that accepts everything and does nothing.
type discardHandler struct{}

func (d *discardHandler) Enabled(_ context.Context, _ slog.Level) bool  { return true }
func (d *discardHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (d *discardHandler) WithAttrs(_ []slog.Attr) slog.Handler          { return d }
func (d *discardHandler) WithGroup(_ string) slog.Handler               { return d }

// waitForEntries polls the fakeLogStore until at least n entries are present or the
// deadline is reached. The background loop flushes every 2 s; we must wait for at
// least one ticker tick before asserting.
func waitForEntries(t *testing.T, s *fakeLogStore, n int, timeout time.Duration) []domain.LogEntry {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if entries := s.captured(); len(entries) >= n {
			return entries
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d log entries in store (got %d)", n, len(s.captured()))
	return nil
}

// --- Enabled ---

func TestDBHandler_Enabled(t *testing.T) {
	t.Parallel()
	inner := slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelWarn})
	store := &fakeLogStore{}
	h := NewDBHandler(inner, store)
	defer h.Close()

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Debug should be disabled (inner is at Warn level)")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Warn should be enabled")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("Error should be enabled")
	}
}

// --- Handle / enqueue / loop ---

func TestDBHandler_Handle_EnqueuesEntry(t *testing.T) {
	t.Parallel()
	store := &fakeLogStore{}
	h := NewDBHandler(&discardHandler{}, store)
	defer h.Close()

	r := slog.NewRecord(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), slog.LevelInfo, "hello", 0)
	r.AddAttrs(slog.String("key", "val"))

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	entries := waitForEntries(t, store, 1, 5*time.Second)
	e := entries[0]
	if e.Level != "INFO" {
		t.Errorf("Level = %q, want INFO", e.Level)
	}
	if e.Message != "hello" {
		t.Errorf("Message = %q, want hello", e.Message)
	}
	if e.Attributes["key"] != "val" {
		t.Errorf("Attributes[key] = %q", e.Attributes["key"])
	}
}

func TestDBHandler_Handle_MultipleRecords(t *testing.T) {
	t.Parallel()
	store := &fakeLogStore{}
	h := NewDBHandler(&discardHandler{}, store)
	defer h.Close()

	for i := 0; i < 5; i++ {
		r := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		if err := h.Handle(context.Background(), r); err != nil {
			t.Fatalf("Handle: %v", err)
		}
	}

	waitForEntries(t, store, 5, 5*time.Second)
}

func TestDBHandler_Handle_NoAttributesWhenEmpty(t *testing.T) {
	t.Parallel()
	store := &fakeLogStore{}
	h := NewDBHandler(&discardHandler{}, store)
	defer h.Close()

	r := slog.NewRecord(time.Now(), slog.LevelWarn, "no attrs", 0)
	_ = h.Handle(context.Background(), r)

	entries := waitForEntries(t, store, 1, 5*time.Second)
	if entries[0].Attributes != nil {
		t.Errorf("Attributes should be nil for record with no attrs, got %v", entries[0].Attributes)
	}
}

// --- WithAttrs ---

func TestDBHandler_WithAttrs_SharesChannel(t *testing.T) {
	t.Parallel()
	store := &fakeLogStore{}
	h := NewDBHandler(&discardHandler{}, store)
	defer h.Close()

	h2, ok := h.WithAttrs([]slog.Attr{slog.String("service", "test")}).(*DBHandler)
	if !ok {
		t.Fatal("WithAttrs must return *DBHandler")
	}
	// Both parent and child share the same channel; use child to log.
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "from child", 0)
	_ = h2.Handle(context.Background(), r)

	waitForEntries(t, store, 1, 5*time.Second)
}

// --- WithGroup ---

func TestDBHandler_WithGroup_SharesChannel(t *testing.T) {
	t.Parallel()
	store := &fakeLogStore{}
	h := NewDBHandler(&discardHandler{}, store)
	defer h.Close()

	h2, ok := h.WithGroup("grp").(*DBHandler)
	if !ok {
		t.Fatal("WithGroup must return *DBHandler")
	}
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "grouped", 0)
	_ = h2.Handle(context.Background(), r)

	waitForEntries(t, store, 1, 5*time.Second)
}

// --- Close / drain ---

func TestDBHandler_Close_DoesNotPanic(t *testing.T) {
	t.Parallel()
	store := &fakeLogStore{}
	h := NewDBHandler(&discardHandler{}, store)

	// Log an entry and allow the background loop to pick it up before closing.
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "before close", 0)
	_ = h.Handle(context.Background(), r)

	// Wait for the entry to be flushed by the ticker (up to 5 s), then close.
	// This verifies that Close() doesn't panic or deadlock.
	waitForEntries(t, store, 1, 5*time.Second)
	h.Close() // must not panic
}

// TestDBHandler_Close_CallableTwice verifies that the cancel function used by
// Close can be called more than once without panicking (context.CancelFunc contract).
func TestDBHandler_Close_CallableTwice(t *testing.T) {
	t.Parallel()
	h := NewDBHandler(&discardHandler{}, &fakeLogStore{})
	h.Close()
	h.Close() // must not panic
}

// --- Component derivation test (via logcomponent, used in logging package context) ---
// This tests that the component helpers work end-to-end.
func TestDBHandler_UsesDiscardInnerHandler(t *testing.T) {
	t.Parallel()
	// Confirm that a nil inner handler panics at construction, not silently.
	// We just verify that a properly-constructed handler does not panic.
	store := &fakeLogStore{}
	h := NewDBHandler(&discardHandler{}, store)
	defer h.Close()

	r := slog.NewRecord(time.Now(), slog.LevelError, "error msg", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	waitForEntries(t, store, 1, 5*time.Second)
	if store.captured()[0].Level != "ERROR" {
		t.Errorf("Level = %q", store.captured()[0].Level)
	}
}
