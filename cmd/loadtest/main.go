// Command loadtest opens N concurrent WebSocket connections against a
// running instance (or the nginx-fronted 2-instance Compose setup),
// subscribes them all to one channel, publishes M events via the
// server-to-server publish API, and reports connection setup time and
// end-to-end delivery latency. Fill the numbers this produces into
// docs/load-test-results.md -- don't hand a client asserted numbers you
// haven't actually measured.
//
// Usage:
//
//	go run ./cmd/loadtest -url ws://localhost:8080/ws -publish http://localhost:8080/api/v1/publish \
//	  -token <jwt> -key <publish-key> -channel broadcast:loadtest -conns 1000 -messages 50
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type message struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Channel   string    `json:"channel,omitempty"`
	Payload   any       `json:"payload,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

func main() {
	wsURL := flag.String("url", "ws://localhost:8080/ws", "WebSocket endpoint")
	publishURL := flag.String("publish", "http://localhost:8080/api/v1/publish", "Publish endpoint")
	token := flag.String("token", "", "JWT for the broadcast channel (any valid token works for broadcast:*)")
	publishKey := flag.String("key", "dev-only-publish-key", "X-Publish-Key value")
	channel := flag.String("channel", "broadcast:loadtest", "Channel to subscribe/publish on")
	conns := flag.Int("conns", 100, "Number of concurrent connections")
	messages := flag.Int("messages", 20, "Number of messages to publish once all connections are subscribed")
	flag.Parse()

	if *token == "" {
		log.Fatal("-token is required (a valid JWT; broadcast:* channels accept any authenticated user)")
	}

	log.Printf("opening %d connections to %s ...", *conns, *wsURL)
	setupStart := time.Now()

	var (
		wg          sync.WaitGroup
		connectedOK int64
		latencies   = make(chan time.Duration, *conns**messages)
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < *conns; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runClient(ctx, *wsURL, *token, *channel, latencies, &connectedOK)
		}(i)
	}

	// Give connections time to establish and send their subscribe frame.
	time.Sleep(2 * time.Second)
	setupElapsed := time.Since(setupStart)
	log.Printf("connections established: %d/%d in %v", atomic.LoadInt64(&connectedOK), *conns, setupElapsed)

	log.Printf("publishing %d messages to %s ...", *messages, *channel)
	for i := 0; i < *messages; i++ {
		if err := publish(*publishURL, *publishKey, *channel, i); err != nil {
			log.Printf("publish %d failed: %v", i, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Give the last messages time to be delivered before we stop collecting.
	time.Sleep(2 * time.Second)
	cancel()
	wg.Wait()
	close(latencies)

	report(setupElapsed, *conns, latencies)
}

func runClient(ctx context.Context, wsURL, token, channel string, latencies chan<- time.Duration, connectedOK *int64) {
	conn, _, err := websocket.Dial(ctx, wsURL+"?token="+token, nil)
	if err != nil {
		log.Printf("dial failed: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "loadtest done")

	sub := message{Type: "subscribe", Channel: channel}
	if err := wsjson.Write(ctx, conn, sub); err != nil {
		log.Printf("subscribe write failed: %v", err)
		return
	}
	atomic.AddInt64(connectedOK, 1)

	for {
		var msg message
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return // context cancelled or connection closed -- normal at test end
		}
		if msg.Type == "ping" {
			_ = wsjson.Write(ctx, conn, message{Type: "pong"})
			continue
		}
		if msg.Type == "event" {
			if ts, ok := msg.Payload.(map[string]any)["sent_at"].(string); ok {
				sentAt, err := time.Parse(time.RFC3339Nano, ts)
				if err == nil {
					latencies <- time.Since(sentAt)
				}
			}
		}
	}
}

func publish(publishURL, key, channel string, seq int) error {
	body, _ := json.Marshal(map[string]any{
		"channel": channel,
		"payload": map[string]any{
			"seq":     seq,
			"sent_at": time.Now().Format(time.RFC3339Nano),
		},
	})
	req, err := http.NewRequest(http.MethodPost, publishURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Publish-Key", key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func report(setupElapsed time.Duration, conns int, latencies <-chan time.Duration) {
	var all []time.Duration
	for d := range latencies {
		all = append(all, d)
	}
	if len(all) == 0 {
		log.Println("no deliveries recorded -- check token/channel/publish key")
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })

	pct := func(p float64) time.Duration {
		idx := int(float64(len(all)-1) * p)
		return all[idx]
	}

	fmt.Println()
	fmt.Println("=== Load Test Results ===")
	fmt.Printf("Connections:        %d\n", conns)
	fmt.Printf("Connection setup:   %v\n", setupElapsed)
	fmt.Printf("Messages delivered: %d\n", len(all))
	fmt.Printf("p50 latency:        %v\n", pct(0.50))
	fmt.Printf("p90 latency:        %v\n", pct(0.90))
	fmt.Printf("p99 latency:        %v\n", pct(0.99))
	fmt.Printf("max latency:        %v\n", all[len(all)-1])
	fmt.Println()
	fmt.Println("Copy these numbers into docs/load-test-results.md")
}
