package pubsub

import (
	"context"
	"sync"

	"github.com/yourname/realtime-notify/internal/hub"
)

// InMemory is a PubSub implementation with no external dependencies.
// It exists for two reasons: fast unit tests without a live Redis, and
// as a local single-instance fallback if someone wants to run this
// service without standing up Redis first (replay history is not
// supported in this mode -- ReplaySince requires the Redis Stream).
//
// It satisfies the same PubSub interface as RedisPubSub, so nothing in
// hub or transport needs to know which one it's talking to.
type InMemory struct {
	mu   sync.RWMutex
	subs map[string][]func(hub.Message)
}

func NewInMemory() *InMemory {
	return &InMemory{subs: make(map[string][]func(hub.Message))}
}

func (m *InMemory) Publish(ctx context.Context, channel string, msg hub.Message) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, fn := range m.subs[channel] {
		fn(msg)
	}
	return nil
}

func (m *InMemory) Subscribe(ctx context.Context, channel string, onMessage func(hub.Message)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subs[channel] = append(m.subs[channel], onMessage)
	return nil
}

func (m *InMemory) Unsubscribe(ctx context.Context, channel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subs, channel)
	return nil
}

func (m *InMemory) Close() error { return nil }
