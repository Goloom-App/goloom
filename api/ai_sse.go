package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/sse"
)

const aiJobStreamReplayLimit = 1000

func (a *API) handleAIJobStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	teamID := r.PathValue("teamID")
	jobs, err := a.store.ListAIJobs(r.Context(), teamID, aiJobStreamReplayLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	events, unsubscribe := a.hub.Subscribe(teamID, strings.TrimSpace(r.Header.Get("Last-Event-ID")))
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for _, job := range jobs {
		if !isActiveAIJob(job.Status) {
			continue
		}
		event, err := aiJobStreamEvent(job)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := event.Write(w); err != nil {
			return
		}
		flusher.Flush()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := event.Write(w); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func aiJobStreamEvent(job domain.AIJob) (sse.Event, error) {
	data, err := json.Marshal(job)
	if err != nil {
		return sse.Event{}, err
	}

	eventType := "job:status"
	if job.Status == domain.AIJobStatusCompleted || job.Status == domain.AIJobStatusFailed {
		eventType = "job:result"
	}

	return sse.Event{
		ID:   job.ID,
		Type: eventType,
		Data: string(data),
	}, nil
}

func isActiveAIJob(status domain.AIJobStatus) bool {
	return status == domain.AIJobStatusPending || status == domain.AIJobStatusProcessing
}
