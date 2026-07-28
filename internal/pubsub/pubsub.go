package pubsub

import (
	"context"

	"github.com/yourname/realtime-notify/internal/hub"
)

// PubSub is the boundary between "how messages move between server
// instances" and everything else. Hub and the transport layer only ever
// see this interface, never redis.Client directly -- swapping Redis for
// NATS or Kafka later means writing one new file, not touching hub.go.
type PubSub interface {
	Publish(ctx context.Context, channel string, msg hub.Message) error
	Subscribe(ctx context.Context, channel string, onMessage func(hub.Message)) error
	Unsubscribe(ctx context.Context, channel string) error
	Close() error
}
