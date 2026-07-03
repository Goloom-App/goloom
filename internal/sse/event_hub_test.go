package sse

import (
	"strings"
	"testing"
	"time"
)

// ---- Event.Write ----

func TestEventWrite_AllFields(t *testing.T) {
	e := Event{
		ID:    "evt-1",
		Type:  "job:status",
		Retry: 3000,
		Data:  `{"status":"done"}`,
	}
	var sb strings.Builder
	if err := e.Write(&sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"id: evt-1\n", "event: job:status\n", "retry: 3000\n", `data: {"status":"done"}` + "\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("event output missing %q:\n%s", want, out)
		}
	}
	// SSE events must end with a blank line.
	if !strings.HasSuffix(out, "\n\n") {
		t.Fatalf("event output must end with double newline, got:\n%q", out)
	}
}

func TestEventWrite_MinimalEvent(t *testing.T) {
	e := Event{Data: "ping"}
	var sb strings.Builder
	if err := e.Write(&sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	// No id, event, retry fields.
	if strings.Contains(out, "id:") || strings.Contains(out, "event:") || strings.Contains(out, "retry:") {
		t.Fatalf("minimal event should not emit optional fields:\n%q", out)
	}
	if !strings.Contains(out, "data: ping\n") {
		t.Fatalf("minimal event missing data line:\n%q", out)
	}
}

func TestEventWrite_MultilineDataEachLinePrefixed(t *testing.T) {
	e := Event{Data: "line one\nline two\nline three"}
	var sb strings.Builder
	if err := e.Write(&sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"data: line one\n", "data: line two\n", "data: line three\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("multiline event missing %q:\n%s", want, out)
		}
	}
}

func TestEventWrite_EmptyDataEmitsDataLine(t *testing.T) {
	e := Event{Data: ""}
	var sb strings.Builder
	if err := e.Write(&sb); err != nil {
		t.Fatal(err)
	}
	// Empty data still requires a "data: \n" line (SSE spec: data field is always present).
	if !strings.Contains(sb.String(), "data: \n") {
		t.Fatalf("empty data should emit 'data: \\n':\n%q", sb.String())
	}
}

// ---- NewHub ----

func TestNewHub_ReturnsNonNilHub(t *testing.T) {
	h := NewHub()
	if h == nil {
		t.Fatal("NewHub returned nil")
	}
	t.Cleanup(h.Close)
}

func TestNewHub_HasDefaultHeartbeatInterval(t *testing.T) {
	h := NewHub()
	t.Cleanup(h.Close)
	if h.heartbeatInterval != defaultHeartbeatInterval {
		t.Fatalf("heartbeatInterval = %v, want %v", h.heartbeatInterval, defaultHeartbeatInterval)
	}
}

func TestNewHubWithZeroIntervalUsesDefault(t *testing.T) {
	h := newHubWithHeartbeatInterval(0)
	t.Cleanup(h.Close)
	if h.heartbeatInterval <= 0 {
		t.Fatalf("zero interval should fall back to default, got %v", h.heartbeatInterval)
	}
}

// ---- Hub.Close idempotence ----

func TestHub_CloseIdempotent(t *testing.T) {
	h := newHubWithHeartbeatInterval(time.Hour)
	h.Close()
	// Second Close must not panic.
	h.Close()
}

// ---- Hub.Publish on closed hub ----

func TestHub_PublishOnClosedHubIsNoop(t *testing.T) {
	h := newHubWithHeartbeatInterval(time.Hour)
	events, unsubscribe := h.Subscribe("team-1", "")
	defer unsubscribe()

	h.Close()
	// Publish after close must not panic.
	h.Publish("team-1", Event{ID: "after-close", Type: "test", Data: "{}"})

	// The channel was closed by Close; no new events should arrive.
	select {
	case evt, ok := <-events:
		if ok {
			t.Fatalf("unexpected event after hub close: %+v", evt)
		}
		// Channel closed: expected.
	case <-time.After(50 * time.Millisecond):
		// No event and channel not closed yet: also acceptable timing-wise.
	}
}

// ---- Hub.Subscribe on closed hub ----

func TestHub_SubscribeOnClosedHub(t *testing.T) {
	h := newHubWithHeartbeatInterval(time.Hour)
	h.Close()

	events, unsubscribe := h.Subscribe("team-1", "")
	defer unsubscribe()

	// The channel should be closed immediately since the hub is already closed.
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("expected closed channel for subscribe-on-closed-hub")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for closed channel from closed hub")
	}
}

// ---- Hub.Publish to different teams ----

func TestHub_PublishDoesNotCrossTeams(t *testing.T) {
	h := newHubWithHeartbeatInterval(time.Hour)
	t.Cleanup(h.Close)

	team1Events, unsub1 := h.Subscribe("team-1", "")
	team2Events, unsub2 := h.Subscribe("team-2", "")
	defer unsub1()
	defer unsub2()

	h.Publish("team-1", Event{ID: "t1-evt", Type: "ping", Data: "{}"})

	// team-1 must receive the event.
	select {
	case got := <-team1Events:
		if got.ID != "t1-evt" {
			t.Fatalf("team1 event ID = %q, want t1-evt", got.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("team-1 timed out waiting for event")
	}

	// team-2 must NOT receive it.
	select {
	case evt := <-team2Events:
		t.Fatalf("team-2 received event that was published to team-1: %+v", evt)
	case <-time.After(50 * time.Millisecond):
		// Expected: no cross-team delivery.
	}
}

// ---- Hub.Publish to nonexistent team ----

func TestHub_PublishToNonexistentTeamNoPanic(t *testing.T) {
	h := newHubWithHeartbeatInterval(time.Hour)
	t.Cleanup(h.Close)
	// Should not panic.
	h.Publish("no-such-team", Event{ID: "x", Data: "{}"})
}
