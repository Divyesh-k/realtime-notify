package hub

import (
	"context"
	"testing"
	"time"
)

func TestClientSendAndOutbox(t *testing.T) {
	c := NewClient("user-1", "org-1")
	if ok := c.Send(Message{Type: TypeEvent, Payload: "hi"}); !ok {
		t.Fatal("send should succeed on open client")
	}
	select {
	case msg := <-c.Outbox():
		if msg.Payload != "hi" {
			t.Fatalf("unexpected payload: %v", msg.Payload)
		}
	default:
		t.Fatal("expected message in outbox")
	}
}

func TestClientBufferFullDropsRatherThanBlocks(t *testing.T) {
	c := NewClient("user-1", "org-1")
	// Fill the buffer without draining it.
	for i := 0; i < outboxSize; i++ {
		if ok := c.Send(Message{Type: TypeEvent}); !ok {
			t.Fatalf("send %d should have succeeded while buffer has room", i)
		}
	}
	// One more should be dropped, not block.
	done := make(chan bool, 1)
	go func() { done <- c.Send(Message{Type: TypeEvent}) }()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("expected send to report failure when buffer is full")
		}
	case <-time.After(time.Second):
		t.Fatal("send blocked instead of dropping -- backpressure guarantee broken")
	}
}

func TestClientSubscriptionTracking(t *testing.T) {
	c := NewClient("user-1", "org-1")
	c.addSubscription("org:org-1")

	found := false
	for _, ch := range c.Subscriptions() {
		if ch == "org:org-1" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected subscribed")
	}

	c.removeSubscription("org:org-1")
	for _, ch := range c.Subscriptions() {
		if ch == "org:org-1" {
			t.Fatal("expected not subscribed after removal")
		}
	}
}

func TestPongUpdatesLivenessAndPreventsTimeout(t *testing.T) {
	c := NewClient("user-1", "org-1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Drain the outbox so Heartbeat's own Send() calls never fill the
	// buffer and trigger a false "dead client" read via that path --
	// we want to isolate the Pong/lastPong liveness check specifically.
	go func() {
		for range c.Outbox() {
		}
	}()

	timedOut := make(chan struct{})
	go c.Heartbeat(ctx, 20*time.Millisecond, func() { close(timedOut) })

	// Keep the client alive by Pong-ing faster than the 2x-interval
	// deadline the heartbeat enforces.
	stop := time.After(150 * time.Millisecond)
loop:
	for {
		select {
		case <-stop:
			break loop
		case <-time.After(10 * time.Millisecond):
			c.Pong()
		}
	}

	select {
	case <-timedOut:
		t.Fatal("heartbeat should not have timed out while Pong() kept liveness fresh")
	default:
		// expected: no timeout
	}
}

func TestHeartbeatTimesOutWithoutPong(t *testing.T) {
	c := NewClient("user-1", "org-1")
	// Force lastPong far in the past so the very first tick sees a stale peer.
	c.lastPong.Store(time.Now().Add(-time.Hour).UnixNano())

	ctx := context.Background()
	timedOut := make(chan struct{})
	go c.Heartbeat(ctx, 10*time.Millisecond, func() { close(timedOut) })

	select {
	case <-timedOut:
		// expected
	case <-time.After(time.Second):
		t.Fatal("expected heartbeat to time out for a client with a stale lastPong")
	}
}
