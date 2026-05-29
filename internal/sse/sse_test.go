package sse

import (
	"sync"
	"testing"
	"time"
)

func TestSSESubscribePublishUnsubscribe(t *testing.T) {
	hub := newHubWithHeartbeatInterval(time.Hour)
	t.Cleanup(hub.Close)

	events, unsubscribe := hub.Subscribe("team-1", "")
	hub.Publish("team-1", Event{ID: "job-1", Type: "job:status", Data: `{"status":"pending"}`})

	select {
	case got := <-events:
		if got.ID != "job-1" {
			t.Fatalf("event ID = %q, want job-1", got.ID)
		}
		if got.Type != "job:status" {
			t.Fatalf("event type = %q, want job:status", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published event")
	}

	unsubscribe()
	if _, ok := <-events; ok {
		t.Fatal("expected client channel to be closed after unsubscribe")
	}
}

func TestSSEHeartbeat(t *testing.T) {
	hub := newHubWithHeartbeatInterval(10 * time.Millisecond)
	t.Cleanup(hub.Close)

	events, unsubscribe := hub.Subscribe("team-1", "")
	defer unsubscribe()

	select {
	case got := <-events:
		if got.Type != "heartbeat" {
			t.Fatalf("event type = %q, want heartbeat", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for heartbeat")
	}
}

func TestSSEConcurrentClients(t *testing.T) {
	hub := newHubWithHeartbeatInterval(time.Hour)
	t.Cleanup(hub.Close)

	const clientCount = 8
	clients := make([]chan Event, 0, clientCount)
	for range clientCount {
		events, unsubscribe := hub.Subscribe("team-1", "")
		clients = append(clients, events)
		defer unsubscribe()
	}

	hub.Publish("team-1", Event{ID: "job-2", Type: "job:result", Data: `{"status":"completed"}`})

	var wg sync.WaitGroup
	for i, events := range clients {
		wg.Add(1)
		go func(idx int, ch chan Event) {
			defer wg.Done()
			select {
			case got := <-ch:
				if got.ID != "job-2" {
					t.Errorf("client %d event ID = %q, want job-2", idx, got.ID)
				}
			case <-time.After(time.Second):
				t.Errorf("client %d timed out waiting for event", idx)
			}
		}(i, events)
	}
	wg.Wait()
}
