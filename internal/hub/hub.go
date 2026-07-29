// Package hub manages connections that are live on THIS server process
// and answers "who's connected to me right now." Cross-instance fan-out
// is internal/pubsub's job — Hub only ever does local delivery, and it's
// pubsub's redis subscription callback that turns "local" on every
// instance into "everyone" from the client's point of view.
package hub

import (
	"context"
	"sync"
	"sync/atomic"
)

// PubSub is declared here, not imported from internal/pubsub, on purpose.
// internal/pubsub imports hub (it needs hub.Message), so if hub also
// imported internal/pubsub for its concrete type, that would be a
// circular import. Declaring the interface hub actually needs — using
// hub's own Message type — lets pubsub.RedisPubSub satisfy it
// structurally with zero coupling back to the pubsub package.
type PubSub interface {
	Publish(ctx context.Context, channel string, msg Message) error
	Subscribe(ctx context.Context, channel string, onMessage func(Message)) error
	Unsubscribe(ctx context.Context, channel string) error
}

type Hub struct {
	mu       sync.RWMutex
	channels map[string]map[*Client]bool
	clients  map[string]*Client

	ps PubSub

	delivered atomic.Int64
	dropped   atomic.Int64
	connTotal atomic.Int64
}

func New(ps PubSub) *Hub {
	return &Hub{
		channels: make(map[string]map[*Client]bool),
		clients:  make(map[string]*Client),
		ps:       ps,
	}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	h.clients[c.ID] = c
	h.mu.Unlock()
	h.connTotal.Add(1)
}

// Unregister removes a client from every channel it was on and closes its
// outbox. Idempotent-ish: calling it twice on the same client is safe
// because the second pass finds nothing left to clean up.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	for ch := range c.subscriptions {
		if set, ok := h.channels[ch]; ok {
			delete(set, c)
			if len(set) == 0 {
				delete(h.channels, ch)
			}
		}
	}
	if _, ok := h.clients[c.ID]; !ok {
		h.mu.Unlock()
		return
	}
	delete(h.clients, c.ID)
	h.mu.Unlock()

	c.close()
}

// SubscribeLocal adds a client to a channel's local delivery set.
// Authorization (channel.CanSubscribe) and the corresponding Redis-level
// subscription both happen in the transport layer before this is called —
// Hub itself stays policy-free.
func (h *Hub) SubscribeLocal(c *Client, channel string) {
	h.mu.Lock()
	if h.channels[channel] == nil {
		h.channels[channel] = make(map[*Client]bool)
	}
	h.channels[channel][c] = true
	h.mu.Unlock()
	c.addSubscription(channel)
}

func (h *Hub) UnsubscribeLocal(c *Client, channel string) {
	h.mu.Lock()
	if set, ok := h.channels[channel]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.channels, channel)
		}
	}
	h.mu.Unlock()
	c.removeSubscription(channel)
}

// DeliverLocal fans a message out to every client on THIS instance
// subscribed to channel. It's the receiving end of pubsub's Redis
// callback — never call it directly for cross-instance delivery, that's
// what Publish is for.
func (h *Hub) DeliverLocal(channel string, msg Message) int {
	h.mu.RLock()
	set := h.channels[channel]
	recipients := make([]*Client, 0, len(set))
	for c := range set {
		recipients = append(recipients, c)
	}
	h.mu.RUnlock()

	delivered := 0
	for _, c := range recipients {
		if c.Send(msg) {
			delivered++
			h.delivered.Add(1)
		} else {
			h.dropped.Add(1)
		}
	}
	return delivered
}

// Publish sends an event to every instance (including this one, via its
// own Redis subscription if it has local subscribers) through the
// configured PubSub backend. This is the entry point server-to-server
// callers (transport.PublishHandler) use.
func (h *Hub) Publish(ctx context.Context, channel string, msg Message) error {
	return h.ps.Publish(ctx, channel, msg)
}

func (h *Hub) LocalConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) LocalChannelCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.channels)
}

func (h *Hub) DeliveredTotal() int64  { return h.delivered.Load() }
func (h *Hub) DroppedTotal() int64    { return h.dropped.Load() }
func (h *Hub) ConnectionsTotal() int64 { return h.connTotal.Load() }
