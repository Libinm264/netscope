// Package pubsub provides a publish-subscribe hub for real-time flow events.
// The Hub interface allows swapping InMemoryHub (single pod) for a Redis-backed
// implementation without changing caller code.
package pubsub

import "sync"

// Hub is the interface for broadcasting flow events to SSE subscribers.
type Hub interface {
	Subscribe(id string) chan []byte
	Unsubscribe(id string)
	Broadcast(data []byte)
}

// ── InMemoryHub ───────────────────────────────────────────────────────────────

// InMemoryHub is a thread-safe, channel-based pub/sub hub.
// Suitable for single-pod deployments; swap for RedisHub for multi-pod HA.
type InMemoryHub struct {
	mu   sync.RWMutex
	subs map[string]chan []byte
}

// NewInMemoryHub creates a ready-to-use InMemoryHub.
func NewInMemoryHub() *InMemoryHub {
	return &InMemoryHub{subs: make(map[string]chan []byte)}
}

func (h *InMemoryHub) Subscribe(id string) chan []byte {
	ch := make(chan []byte, 128)
	h.mu.Lock()
	h.subs[id] = ch
	h.mu.Unlock()
	return ch
}

func (h *InMemoryHub) Unsubscribe(id string) {
	h.mu.Lock()
	if ch, ok := h.subs[id]; ok {
		close(ch)
		delete(h.subs, id)
	}
	h.mu.Unlock()
}

func (h *InMemoryHub) Broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.subs {
		select {
		case ch <- data:
		default:
			// Slow consumer — drop rather than block
		}
	}
}

// SubscriberCount returns the number of active SSE connections (useful for metrics).
func (h *InMemoryHub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}
