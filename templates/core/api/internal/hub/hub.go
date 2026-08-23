// Package hub - in-process fanout for the `events` subscription.
// Parallel fanouts: mutation broadcast hits this hub AND fires
// Realtime.Broadcast over gRPC; neither depends on the other.
package hub

import "sync"

type Hub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
	buf  int
}

func New(buf int) *Hub {
	return &Hub{subs: map[chan string]struct{}{}, buf: buf}
}

func (h *Hub) Subscribe() chan string {
	ch := make(chan string, h.buf)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *Hub) Broadcast(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default: // slow subscriber: drop rather than block the mutation
		}
	}
}
