package service

import (
	"sync"
	"time"
)

// Event is a real-time notification pushed to connected UIs over SSE.
type Event struct {
	Type       string    `json:"type"` // telemetry | lifecycle
	ProjectID  uint      `json:"projectId"`
	DeviceID   uint      `json:"deviceId"`
	DeviceKey  string    `json:"deviceKey"`
	Identifier string    `json:"identifier,omitempty"`
	Value      any       `json:"value,omitempty"`
	Online     *bool     `json:"online,omitempty"`
	Time       time.Time `json:"time"`
}

// Hub is a tiny in-process fan-out bus for SSE clients. Slow subscribers are
// dropped rather than blocking ingestion.
type Hub struct {
	mu   sync.RWMutex
	subs map[int]chan Event
	next int
}

func NewHub() *Hub {
	return &Hub{subs: map[int]chan Event{}}
}

// Subscribe returns a subscriber id and a buffered channel.
func (h *Hub) Subscribe() (int, chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	id := h.next
	ch := make(chan Event, 32)
	h.subs[id] = ch
	return id, ch
}

func (h *Hub) Unsubscribe(id int) {
	h.mu.Lock()
	if ch, ok := h.subs[id]; ok {
		delete(h.subs, id)
		close(ch)
	}
	h.mu.Unlock()
}

// Publish delivers an event to all subscribers without blocking.
func (h *Hub) Publish(e Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.subs {
		select {
		case ch <- e:
		default:
			// subscriber is slow; drop this event for it
		}
	}
}
