package api

import (
	"testing"
	"time"
)

func TestEventBusPublishesAndUnsubscribes(t *testing.T) {
	bus := NewEventBus()
	ch, unsubscribe := bus.Subscribe()
	bus.Publish("device.created", map[string]string{"id": "one"})
	select {
	case event := <-ch:
		if event.Type != "device.created" {
			t.Fatalf("unexpected event type %q", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("event was not published")
	}
	unsubscribe()
}

func TestEventJSON(t *testing.T) {
	got := string(eventJSON(Event{Type: "check.result", Data: map[string]bool{"up": true}}))
	want := `{"type":"check.result","data":{"up":true}}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
