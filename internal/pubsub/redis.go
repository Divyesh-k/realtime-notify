package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/yourname/realtime-notify/internal/hub"
)

const channelPrefix = "rtn:"

// RedisPubSub fans messages out across every server instance subscribed
// to the same channel. Each instance runs its own *redis.PubSub per
// channel with its own goroutine reading from it -- Redis handles the
// actual broadcast, we just bridge it into hub.Message callbacks.
type RedisPubSub struct {
	client *redis.Client

	mu   sync.Mutex
	subs map[string]*redis.PubSub
}

func NewRedis(client *redis.Client) *RedisPubSub {
	return &RedisPubSub{
		client: client,
		subs:   make(map[string]*redis.PubSub),
	}
}

func (r *RedisPubSub) Publish(ctx context.Context, channel string, msg hub.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if err := r.client.Publish(ctx, channelPrefix+channel, data).Err(); err != nil {
		return fmt.Errorf("redis publish: %w", err)
	}
	if err := appendReplay(ctx, r.client, channel, data); err != nil {
		slog.Warn("failed to append replay entry", "channel", channel, "err", err)
	}
	return nil
}

func (r *RedisPubSub) Subscribe(ctx context.Context, channel string, onMessage func(hub.Message)) error {
	r.mu.Lock()
	if _, exists := r.subs[channel]; exists {
		r.mu.Unlock()
		return nil
	}
	sub := r.client.Subscribe(ctx, channelPrefix+channel)
	r.subs[channel] = sub
	r.mu.Unlock()

	if _, err := sub.Receive(ctx); err != nil {
		return fmt.Errorf("redis subscribe confirm: %w", err)
	}

	go func() {
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case raw, ok := <-ch:
				if !ok {
					return
				}
				var msg hub.Message
				if err := json.Unmarshal([]byte(raw.Payload), &msg); err != nil {
					slog.Error("failed to unmarshal pubsub message", "channel", channel, "err", err)
					continue
				}
				onMessage(msg)
			}
		}
	}()

	return nil
}

func (r *RedisPubSub) Unsubscribe(ctx context.Context, channel string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub, ok := r.subs[channel]
	if !ok {
		return nil
	}
	delete(r.subs, channel)
	return sub.Close()
}

func (r *RedisPubSub) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for ch, sub := range r.subs {
		_ = sub.Close()
		delete(r.subs, ch)
	}
	return r.client.Close()
}
