package sse

import (
	"sync"
	"time"
)

const defaultHeartbeatInterval = 25 * time.Second

type Hub struct {
	mu                sync.RWMutex
	clients           map[string]map[*Client]struct{}
	closed            bool
	heartbeatInterval time.Duration
}

func NewHub() *Hub {
	return newHubWithHeartbeatInterval(defaultHeartbeatInterval)
}

func newHubWithHeartbeatInterval(interval time.Duration) *Hub {
	if interval <= 0 {
		interval = defaultHeartbeatInterval
	}
	return &Hub{
		clients:           make(map[string]map[*Client]struct{}),
		heartbeatInterval: interval,
	}
}

func (h *Hub) Subscribe(teamID, lastEventID string) (chan Event, func()) {
	client := newClient(teamID, lastEventID, h.heartbeatInterval)

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		client.Close()
		return client.Events(), func() {}
	}
	if h.clients[teamID] == nil {
		h.clients[teamID] = make(map[*Client]struct{})
	}
	h.clients[teamID][client] = struct{}{}
	h.mu.Unlock()

	return client.Events(), func() {
		h.removeClient(teamID, client)
	}
}

func (h *Hub) Publish(teamID string, event Event) {
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return
	}
	teamClients := make([]*Client, 0, len(h.clients[teamID]))
	for client := range h.clients[teamID] {
		teamClients = append(teamClients, client)
	}
	h.mu.RUnlock()

	for _, client := range teamClients {
		if !client.Send(event) {
			h.removeClient(teamID, client)
		}
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	clients := h.clients
	h.clients = make(map[string]map[*Client]struct{})
	h.mu.Unlock()

	for _, teamClients := range clients {
		for client := range teamClients {
			client.Close()
		}
	}
}

func (h *Hub) removeClient(teamID string, client *Client) {
	h.mu.Lock()
	teamClients := h.clients[teamID]
	if teamClients != nil {
		delete(teamClients, client)
		if len(teamClients) == 0 {
			delete(h.clients, teamID)
		}
	}
	h.mu.Unlock()
	client.Close()
}
