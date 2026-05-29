package sse

import (
	"sync"
	"time"
)

const clientBufferSize = 64

type Client struct {
	TeamID      string
	LastEventID string

	mu     sync.RWMutex
	ch     chan Event
	closed bool
	once   sync.Once
	stopCh chan struct{}
}

func newClient(teamID, lastEventID string, heartbeatInterval time.Duration) *Client {
	client := &Client{
		TeamID:      teamID,
		LastEventID: lastEventID,
		ch:          make(chan Event, clientBufferSize),
		stopCh:      make(chan struct{}),
	}
	go client.runHeartbeat(heartbeatInterval)
	return client
}

func (c *Client) Events() chan Event {
	return c.ch
}

func (c *Client) Send(event Event) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return false
	}

	select {
	case c.ch <- event:
		return true
	default:
		go c.Close()
		return false
	}
}

func (c *Client) Close() {
	c.once.Do(func() {
		close(c.stopCh)
		c.mu.Lock()
		c.closed = true
		close(c.ch)
		c.mu.Unlock()
	})
}

func (c *Client) runHeartbeat(interval time.Duration) {
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			if !c.Send(Event{Type: "heartbeat", Data: "{}"}) {
				return
			}
		}
	}
}
