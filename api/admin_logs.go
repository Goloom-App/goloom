package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) handleListLogEntries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := domain.LogFilter{
		Level:  strings.TrimSpace(q.Get("level")),
		Search: strings.TrimSpace(q.Get("search")),
	}

	if raw := strings.TrimSpace(q.Get("archived")); raw != "" {
		v := raw == "true" || raw == "1"
		filter.Archived = &v
	}
	if raw := strings.TrimSpace(q.Get("before")); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			filter.Before = &t
		}
	}
	if raw := strings.TrimSpace(q.Get("after")); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			filter.After = &t
		}
	}
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
			filter.Limit = n
		}
	}
	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	entries, err := a.store.ListLogEntries(r.Context(), filter)
	if err != nil {
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}
	total, err := a.store.CountLogEntries(r.Context(), filter)
	if err != nil {
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"total":   total,
	})
}

func (a *API) handleArchiveLogEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		a.writeError(w, r, "missing_id", http.StatusBadRequest)
		return
	}
	if err := a.store.ArchiveLogEntry(r.Context(), id); err != nil {
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleUnarchiveLogEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		a.writeError(w, r, "missing_id", http.StatusBadRequest)
		return
	}
	if err := a.store.UnarchiveLogEntry(r.Context(), id); err != nil {
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleDeleteLogEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		a.writeError(w, r, "missing_id", http.StatusBadRequest)
		return
	}
	if err := a.store.DeleteLogEntry(r.Context(), id); err != nil {
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handlePruneLogEntries(w http.ResponseWriter, r *http.Request) {
	before := time.Now().Add(-7 * 24 * time.Hour)
	if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			before = t
		}
	}
	count, err := a.store.DeleteLogEntriesBefore(r.Context(), before)
	if err != nil {
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted_count": count,
	})
}
