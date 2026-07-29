package hub

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakePubSub stands in for Redis in tests: Publish calls DeliverLocal
// directly, simulating a single-instance round trip without a live
// Redis. Cross-instance behavior is pubsub's responsibility and is
// exercised by the Docker Compose two-instance demo, not these tests.
type fakePubSub struct {
	target *Hub
}

func (f *fakePubSub) Publish(ctx context.Context, channel string, msg Message) error {
	f.target.DeliverLocal(channel, msg)
	return nil
}
func (f *fakePubSub) Subscribe(ctx context.Context, channel string, onMessage func(Message)) error {
	return nil
}
func (f *fakePubSub) Unsubscribe(ctx context.Context, channel string) error { return nil }

func newTestHub() *Hub {
	h := New(nil)
	h.ps = &fakePubSub{target: h}
	return h
}

func TestRegisterAndUnregister(t *testing.T) {
	h := newTestHub()
	c := NewClient("user-1", "org-1")

	h.Register(c)
	if got := h.LocalConnectionCount(); got != 1 {
		t.Fatalf("expected 1 connection, got %d", got)
	}

	h.Unregister(c)
	if got := h.LocalConnectionCount(); got != 0 {
		t.Fatalf("expected 0 connections after unregister, got %d", got)
	}
}

func TestSubscribeDeliversToLocalClient(t *testing.T) {
	h := newTestHub()
	c := NewClient("user-1", "org-1")
	h.Register(c)
	defer h.Unregister(c)

	h.SubscribeLocal(c, "org:org-1")
	found := false
	for _, ch := range c.Subscriptions() {
		if ch == "org:org-1" {
			found = true
		}
	}
	if !found {
		t.Fatal("client should be marked subscribed")
	}

	msg := Message{Type: TypeEvent, Channel: "org:org-1", Payload: "hello"}
	if err := h.Publish(context.Background(), "org:org-1", msg); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case got := <-c.Outbox():
		if got.Payload != "hello" {
			t.Fatalf("expected payload 'hello', got %v", got.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivered message")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	h := newTestHub()
	c := NewClient("user-1", "org-1")
	h.Register(c)
	defer h.Unregister(c)

	h.SubscribeLocal(c, "org:org-1")
	h.UnsubscribeLocal(c, "org:org-1")

	for _, ch := range c.Subscriptions() {
		if ch == "org:org-1" {
			t.Fatal("client should not be subscribed after unsubscribe")
		}
	}

	_ = h.Publish(context.Background(), "org:org-1", Message{Type: TypeEvent, Channel: "org:org-1"})

	select {
	case msg := <-c.Outbox():
		t.Fatalf("expected no delivery after unsubscribe, got %v", msg)
	case <-time.After(100 * time.Millisecond):
		// expected: nothing arrives
	}
}

func TestUnregisterCleansUpChannelMembership(t *testing.T) {
	h := newTestHub()
	c1 := NewClient("user-1", "org-1")
	c2 := NewClient("user-2", "org-1")
	h.Register(c1)
	h.Register(c2)
	h.SubscribeLocal(c1, "org:org-1")
	h.SubscribeLocal(c2, "org:org-1")

	h.Unregister(c1)

	if got := h.LocalChannelCount(); got != 1 {
		t.Fatalf("channel should still exist while c2 is subscribed, got %d channels", got)
	}

	h.Unregister(c2)
	if got := h.LocalChannelCount(); got != 0 {
		t.Fatalf("channel should be cleaned up once empty, got %d channels", got)
	}
}

func TestDeliverLocalReturnsCount(t *testing.T) {
	h := newTestHub()
	c1 := NewClient("user-1", "org-1")
	c2 := NewClient("user-2", "org-1")
	h.Register(c1)
	h.Register(c2)
	defer h.Unregister(c1)
	defer h.Unregister(c2)

	h.SubscribeLocal(c1, "org:org-1")
	h.SubscribeLocal(c2, "org:org-1")

	n := h.DeliverLocal("org:org-1", Message{Type: TypeEvent})
	if n != 2 {
		t.Fatalf("expected 2 recipients, got %d", n)
	}
	if got := h.DeliveredTotal(); got != 2 {
		t.Fatalf("expected DeliveredTotal to be 2, got %d", got)
	}
}

func TestConcurrentSubscribeAndPublish(t *testing.T) {
	// Guards against data races in the hub's channel map under concurrent
	// access -- run with `go test -race` to make this test meaningful.
	h := newTestHub()
	var wg sync.WaitGroup

	clients := make([]*Client, 20)
	for i := range clients {
		clients[i] = NewClient("user", "org-1")
		h.Register(clients[i])
	}

	for _, c := range clients {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			h.SubscribeLocal(c, "org:org-1")
		}(c)
	}
	wg.Wait()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.Publish(context.Background(), "org:org-1", Message{Type: TypeEvent})
		}()
	}
	wg.Wait()

	for _, c := range clients {
		h.Unregister(c)
	}
}
