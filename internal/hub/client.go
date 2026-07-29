package hub

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const outboxSize = 32

// Client represents one connected consumer — a WebSocket or an SSE
// stream, the Hub doesn't care which. It deliberately holds no reference
// to a socket or ResponseWriter: the transport layer owns the connection
// and drains Outbox() into it. That separation is what lets the same
// Client/Hub code serve both transports in ws.go and sse.go.
type Client struct {
	ID     string
	UserID string
	OrgID  string

	outbox chan Message

	mu            sync.Mutex
	subscriptions map[string]bool

	lastPong atomic.Int64 // unix nanos, updated on Pong()
}

func NewClient(userID, orgID string) *Client {
	c := &Client{
		ID:            NewID(),
		UserID:        userID,
		OrgID:         orgID,
		outbox:        make(chan Message, outboxSize),
		subscriptions: make(map[string]bool),
	}
	c.lastPong.Store(time.Now().UnixNano())
	return c
}

// Outbox is drained by the transport layer (ws.go's writeLoop, sse.go's
// event loop) and written out to the actual connection.
func (c *Client) Outbox() <-chan Message {
	return c.outbox
}

// Send is a non-blocking enqueue. If the client's buffer is full — a slow
// reader, a dead connection not yet cleaned up — the message is dropped
// rather than blocking the whole hub on one bad consumer. Returns false
// when dropped so callers can count it.
func (c *Client) Send(msg Message) bool {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	select {
	case c.outbox <- msg:
		return true
	default:
		return false
	}
}

// close is called once by Hub.Unregister; closing the outbox unblocks
// whatever transport goroutine is ranging over Outbox().
func (c *Client) close() {
	close(c.outbox)
}

func (c *Client) Pong() {
	c.lastPong.Store(time.Now().UnixNano())
}

func (c *Client) addSubscription(channel string) {
	c.mu.Lock()
	c.subscriptions[channel] = true
	c.mu.Unlock()
}

func (c *Client) removeSubscription(channel string) {
	c.mu.Lock()
	delete(c.subscriptions, channel)
	c.mu.Unlock()
}

func (c *Client) Subscriptions() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.subscriptions))
	for ch := range c.subscriptions {
		out = append(out, ch)
	}
	return out
}

// Heartbeat sends an app-level ping every interval and watches for a Pong
// within two intervals. This is deliberately at the message level (not
// native WS ping/pong frames) because ws.go moves everything through
// wsjson — a JSON-message transport — so liveness has to travel the same
// path as everything else. Calls cancel() and returns once the peer looks
// dead, which unwinds the connection's context and triggers cleanup.
func (c *Client) Heartbeat(ctx context.Context, interval time.Duration, cancel context.CancelFunc) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			last := time.Unix(0, c.lastPong.Load())
			if time.Since(last) > interval*2 {
				cancel()
				return
			}
			c.Send(Message{Type: TypePing})
		}
	}
}
