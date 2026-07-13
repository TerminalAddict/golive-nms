package api

import (
	"encoding/json"
	"sync"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}
type EventBus struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
}

func NewEventBus() *EventBus { return &EventBus{clients: map[chan Event]struct{}{}} }
func (b *EventBus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() { b.mu.Lock(); delete(b.clients, ch); close(ch); b.mu.Unlock() }
}
func (b *EventBus) Publish(t string, d any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for c := range b.clients {
		select {
		case c <- Event{Type: t, Data: d}:
		default:
		}
	}
}
func eventJSON(e Event) []byte { b, _ := json.Marshal(e); return b }
