package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourname/realtime-notify/internal/hub"
)

// Replay design: a client that drops its WebSocket for a few seconds
// (phone locks, wifi hiccup, laptop sleeps) shouldn't silently miss
// whatever happened in that window. We keep a short rolling buffer per
// channel in a Redis Stream and let reconnecting clients ask for
// "everything after the last message I saw."
//
// This is deliberately NOT a durable event log with delivery guarantees
// across days -- it's a small window sized for reconnect gaps, not
// offline clients. A client gone for an hour should refetch state via
// a normal API call, not replay a stream.

const (
	replayMaxLen   = 500
	replayTTL      = 5 * time.Minute
	replayStreamPr = "rtn:replay:"
)

func replayKey(channel string) string {
	return replayStreamPr + channel
}

func appendReplay(ctx context.Context, client *redis.Client, channel string, payload []byte) error {
	key := replayKey(channel)
	pipe := client.TxPipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		MaxLen: replayMaxLen,
		Approx: true,
		Values: map[string]any{"data": payload},
	})
	pipe.Expire(ctx, key, replayTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// ReplaySince returns every message published to channel after
// lastSeenID. If lastSeenID is empty, it returns nothing -- a fresh
// subscriber gets only new messages going forward, not history.
func ReplaySince(ctx context.Context, client *redis.Client, channel, lastSeenID string) ([]hub.Message, error) {
	if lastSeenID == "" {
		return nil, nil
	}
	key := replayKey(channel)
	results, err := client.XRange(ctx, key, "("+lastSeenID, "+").Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("xrange replay: %w", err)
	}

	msgs := make([]hub.Message, 0, len(results))
	for _, entry := range results {
		raw, ok := entry.Values["data"].(string)
		if !ok {
			continue
		}
		var msg hub.Message
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		msg.ID = entry.ID
		msgs = append(msgs, msg)
	}
	return msgs, nil
}
