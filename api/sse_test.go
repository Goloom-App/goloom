package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	internalsse "git.f4mily.net/goloom/internal/sse"
)

type sseEvent struct {
	ID    string
	Type  string
	Data  string
	Retry string
}

func TestSSEEndpoint(t *testing.T) {
	ctx := context.Background()
	store := newAICRUDStore(t)
	hub := internalsse.NewHub()
	t.Cleanup(hub.Close)
	h := newAICRUDHandlerWithManagerAndHub(t, store, nil, hub)

	user, err := store.UpsertOIDCUser(ctx, "sse-user", "sse@example.test", "SSE User")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "sse-team"})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := store.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	bearer, _, err := store.CreateUserAPIToken(ctx, user.ID, "sse-token", nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	processingJob, err := store.CreateAIJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: user.ID,
		Type:         domain.AIJobTypeVoiceEngine,
		Status:       domain.AIJobStatusProcessing,
		Payload:      json.RawMessage(`{"params":{"topic":"status"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingJob, err := store.CreateAIJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: user.ID,
		Type:         domain.AIJobTypeCampaignAutopilot,
		Status:       domain.AIJobStatusPending,
		Payload:      json.RawMessage(`{"params":{"topic":"pending"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAIJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: user.ID,
		Type:         domain.AIJobTypeProactiveTrigger,
		Status:       domain.AIJobStatusCompleted,
		Payload:      json.RawMessage(`{"params":{"topic":"done"}}`),
		Result:       json.RawMessage(`{"draft":"complete"}`),
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(h)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/teams/" + team.ID + "/ai-jobs/stream?token=" + url.QueryEscape(bearer))
	if err != nil {
		t.Fatalf("GET SSE stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}
	if got := resp.Header.Get("Connection"); got != "keep-alive" {
		t.Fatalf("Connection = %q, want keep-alive", got)
	}

	reader := bufio.NewReader(resp.Body)

	first := readSSEEvent(t, reader)
	second := readSSEEvent(t, reader)

	replayed := map[string]sseEvent{first.ID: first, second.ID: second}
	if len(replayed) != 2 {
		t.Fatalf("replayed event count = %d, want 2", len(replayed))
	}
	assertReplayEvent(t, replayed[processingJob.ID], processingJob.ID, domain.AIJobStatusProcessing)
	assertReplayEvent(t, replayed[pendingJob.ID], pendingJob.ID, domain.AIJobStatusPending)

	published := internalsse.Event{ID: "job-result", Type: "job:result", Data: `{"status":"completed"}`}
	hub.Publish(team.ID, published)
	third := readSSEEvent(t, reader)
	if third.ID != published.ID {
		t.Fatalf("published event ID = %q, want %q", third.ID, published.ID)
	}
	if third.Type != published.Type {
		t.Fatalf("published event type = %q, want %q", third.Type, published.Type)
	}
	if third.Data != published.Data {
		t.Fatalf("published event data = %q, want %q", third.Data, published.Data)
	}
}

var errSSEReadTimeout = errors.New("sse read timeout")

func assertReplayEvent(t *testing.T, event sseEvent, wantID string, wantStatus domain.AIJobStatus) {
	t.Helper()
	if event.ID != wantID {
		t.Fatalf("event ID = %q, want %q", event.ID, wantID)
	}
	if event.Type != "job:status" {
		t.Fatalf("event type = %q, want job:status", event.Type)
	}
	var job domain.AIJob
	if err := json.Unmarshal([]byte(event.Data), &job); err != nil {
		t.Fatalf("unmarshal event data: %v", err)
	}
	if job.Status != wantStatus {
		t.Fatalf("job status = %q, want %q", job.Status, wantStatus)
	}
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) sseEvent {
	t.Helper()
	event, err := tryReadSSEEvent(reader, time.Second)
	if err != nil {
		t.Fatalf("read SSE event: %v", err)
	}
	return event
}

func tryReadSSEEvent(reader *bufio.Reader, timeout time.Duration) (sseEvent, error) {
	type result struct {
		event sseEvent
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		var event sseEvent
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				ch <- result{err: err}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				ch <- result{event: event}
				return
			}
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			switch key {
			case "id":
				event.ID = value
			case "event":
				event.Type = value
			case "data":
				if event.Data == "" {
					event.Data = value
				} else {
					event.Data += "\n" + value
				}
			case "retry":
				event.Retry = value
			}
		}
	}()

	select {
	case result := <-ch:
		return result.event, result.err
	case <-time.After(timeout):
		return sseEvent{}, errSSEReadTimeout
	}
}
