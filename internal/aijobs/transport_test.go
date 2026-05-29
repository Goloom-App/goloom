package aijobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestHTTPTransportDeliver(t *testing.T) {
	testHTTPTransportDeliver(t)
}

func TestTransportHTTPDeliver(t *testing.T) {
	testHTTPTransportDeliver(t)
}

func testHTTPTransportDeliver(t *testing.T) {
	t.Helper()

	var received httpDispatchPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: got %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type: got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	transport := &HTTPTransport{}
	job := domain.AIJob{
		ID:           "job-1",
		TeamID:       "team-1",
		AuthorUserID: "user-1",
		Type:         domain.AIJobTypeVoiceEngine,
		Payload: json.RawMessage(`{
			"callback_url":"http://goloom.test/v1/webhooks/ai-callback",
			"params":{"prompt_hint":"ship it"},
			"context":{"team":{"id":"team-1"}}
		}`),
	}

	if err := transport.Dispatch(context.Background(), job, server.URL); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if received.JobID != job.ID {
		t.Fatalf("job_id: got %q, want %q", received.JobID, job.ID)
	}
	if received.Type != job.Type {
		t.Fatalf("type: got %q, want %q", received.Type, job.Type)
	}
	if received.TeamID != job.TeamID {
		t.Fatalf("team_id: got %q, want %q", received.TeamID, job.TeamID)
	}
	if received.AuthorUserID != job.AuthorUserID {
		t.Fatalf("author_user_id: got %q, want %q", received.AuthorUserID, job.AuthorUserID)
	}
	if received.CallbackURL != "http://goloom.test/v1/webhooks/ai-callback" {
		t.Fatalf("callback_url: got %q", received.CallbackURL)
	}
	if string(received.Params) != `{"prompt_hint":"ship it"}` {
		t.Fatalf("params: got %s", string(received.Params))
	}
	if string(received.Context) != `{"team":{"id":"team-1"}}` {
		t.Fatalf("context: got %s", string(received.Context))
	}
}

func TestHTTPTransportRetry(t *testing.T) {
	testHTTPTransportRetry(t)
}

func TestTransportHTTPRetry(t *testing.T) {
	testHTTPTransportRetry(t)
}

func testHTTPTransportRetry(t *testing.T) {
	t.Helper()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	transport := &HTTPTransport{
		RetryBackoffs:  []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
		AttemptTimeout: time.Second,
		sleep: func(context.Context, time.Duration) error {
			return nil
		},
	}

	job := domain.AIJob{
		ID:           "job-2",
		TeamID:       "team-2",
		AuthorUserID: "user-2",
		Type:         domain.AIJobTypeCampaignAutopilot,
		Payload:      json.RawMessage(`{"params":{"campaign_format_id":"cf-1"}}`),
	}

	if err := transport.Dispatch(context.Background(), job, server.URL); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if got := attempts.Load(); got != 3 {
		t.Fatalf("attempts: got %d, want 3", got)
	}
}
