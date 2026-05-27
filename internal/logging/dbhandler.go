package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/store"
)

const logChanCap = 500

// DBHandler wraps an existing slog.Handler and asynchronously persists log
// entries to the database via a LogStore. It never blocks the caller — if the
// internal channel is full, the entry is dropped (counter lost).
type DBHandler struct {
	inner slog.Handler
	store store.LogStore
	ch    chan *domain.LogEntry
	cancel context.CancelFunc
}

// NewDBHandler returns a new slog.Handler that writes to inner (typically
// stdout) and enqueues a copy for database persistence. Caller must call
// Close when the logger is no longer needed.
func NewDBHandler(inner slog.Handler, s store.LogStore) *DBHandler {
	ctx, cancel := context.WithCancel(context.Background())
	h := &DBHandler{
		inner:  inner,
		store:  s,
		ch:     make(chan *domain.LogEntry, logChanCap),
		cancel: cancel,
	}
	go h.loop(ctx)
	return h
}

// Close shuts down the background flusher.
func (h *DBHandler) Close() {
	h.cancel()
}

// Enabled delegates to the inner handler.
func (h *DBHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle writes to the inner handler and asynchronously enqueues a copy.
func (h *DBHandler) Handle(ctx context.Context, r slog.Record) error {
	if err := h.inner.Handle(ctx, r); err != nil {
		return err
	}
	h.enqueue(r)
	return nil
}

// WithAttrs returns a new handler with added attributes (delegates to inner).
func (h *DBHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &DBHandler{
		inner:  h.inner.WithAttrs(attrs),
		store:  h.store,
		ch:     h.ch,
		cancel: h.cancel,
	}
}

// WithGroup returns a new handler with a group (delegates to inner).
func (h *DBHandler) WithGroup(name string) slog.Handler {
	return &DBHandler{
		inner:  h.inner.WithGroup(name),
		store:  h.store,
		ch:     h.ch,
		cancel: h.cancel,
	}
}

// --- internal ---

func (h *DBHandler) enqueue(r slog.Record) {
	e := &domain.LogEntry{
		ID:        "",
		Level:     r.Level.String(),
		Message:   r.Message,
		CreatedAt: r.Time,
	}

	// Source info (set if AddSource is enabled).
	if r.PC != 0 {
		if fs := runtime.FuncForPC(r.PC); fs != nil {
			file, line := fs.FileLine(r.PC)
			e.SourceFile = file
			e.SourceLine = line
		}
	}

	// Attributes.
	attrs := make(map[string]string, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	if len(attrs) > 0 {
		e.Attributes = attrs
	}

	// Non-blocking send — drop if full.
	select {
	case h.ch <- e:
	default:
	}
}

func (h *DBHandler) loop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var batch []*domain.LogEntry
	flush := func() {
		if len(batch) == 0 {
			return
		}
		for _, e := range batch {
			if err := h.store.InsertLogEntry(context.Background(), *e); err != nil {
				// Log failure to stdout via the inner handler (best effort).
				attrs, _ := json.Marshal(e.Attributes)
				h.inner.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelError,
					"failed to persist log entry", 0))
				_ = attrs // swallow — we cannot retry easily here
			}
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case e := <-h.ch:
			batch = append(batch, e)
			if len(batch) >= 50 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
