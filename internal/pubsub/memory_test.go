package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/yourname/realtime-notify/internal/hub"
)

func TestInMemoryPublishSubscribe(t *testing.T) {
	ps := NewInMemory()
	received := make(chan hub.Message, 1)

	if err := ps.Subscribe(context.Background(), "org:1", func(m hub.Message) {
		received <- m
	}); err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	msg := hub.Message{Type: hub.TypeEvent, Channel: "org:1", Payload: "hi"}
	if err := ps.Publish(context.Background(), "org:1", msg); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case got := <-received:
		if got.Payload != "hi" {
			t.Fatalf("unexpected payload: %v", got.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestInMemoryUnsubscribeStopsDelivery(t *testing.T) {
	ps := NewInMemory()
	received := make(chan hub.Message, 1)

	_ = ps.Subscribe(context.Background(), "org:1", func(m hub.Message) { received <- m })
	_ = ps.Unsubscribe(context.Background(), "org:1")
	_ = ps.Publish(context.Background(), "org:1", hub.Message{Type: hub.TypeEvent})

	select {
	case msg := <-received:
		t.Fatalf("expected no delivery after unsubscribe, got %v", msg)
	case <-time.After(100 * time.Millisecond):
		// expected
	}
}

func TestInMemoryMultipleSubscribersAllReceive(t *testing.T) {
	ps := NewInMemory()
	a := make(chan hub.Message, 1)
	b := make(chan hub.Message, 1)

	_ = ps.Subscribe(context.Background(), "org:1", func(m hub.Message) { a <- m })
	_ = ps.Subscribe(context.Background(), "org:1", func(m hub.Message) { b <- m })

	_ = ps.Publish(context.Background(), "org:1", hub.Message{Type: hub.TypeEvent, Payload: "fan-out"})

	for name, ch := range map[string]chan hub.Message{"a": a, "b": b} {
		select {
		case msg := <-ch:
			if msg.Payload != "fan-out" {
				t.Fatalf("subscriber %s got wrong payload: %v", name, msg.Payload)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %s did not receive message", name)
		}
	}
}
