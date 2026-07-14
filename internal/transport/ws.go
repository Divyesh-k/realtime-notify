package transport

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/yourname/realtime-notify/internal/auth"
	"github.com/yourname/realtime-notify/internal/channel"
	"github.com/yourname/realtime-notify/internal/hub"
	"github.com/yourname/realtime-notify/internal/pubsub"
)

const (
	heartbeatInterval = 30 * time.Second
	writeTimeout      = 10 * time.Second
)

type WSHandler struct {
	hub            *hub.Hub
	ps             pubsub.PubSub
	verifier       *auth.Verifier
	redis          *redis.Client // used directly only for replay reads; nil in memory-driver mode
	allowedOrigins []string
}

func NewWSHandler(h *hub.Hub, ps pubsub.PubSub, v *auth.Verifier, redisClient *redis.Client, allowedOrigins []string) *WSHandler {
	return &WSHandler{hub: h, ps: ps, verifier: v, redis: redisClient, allowedOrigins: allowedOrigins}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	claims, err := h.verifier.Verify(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Driven by ALLOWED_ORIGINS ("*" only in dev -- config.Load
		// refuses to start in production without an explicit list).
		OriginPatterns: h.allowedOrigins,
	})
	if err != nil {
		slog.Error("websocket accept failed", "err", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	client := hub.NewClient(claims.UserID, claims.OrgID)
	h.hub.Register(client)
	defer h.hub.Unregister(client)
	defer conn.Close(websocket.StatusNormalClosure, "closing")

	// Writer goroutine: the ONLY goroutine allowed to write to this
	// connection. gorilla/nhooyr websocket connections aren't safe for
	// concurrent writes, so every outbound message — hub broadcasts,
	// pings, acks, errors — funnels through client.Outbox().
	go h.writeLoop(ctx, conn, client)

	go client.Heartbeat(ctx, heartbeatInterval, cancel)

	h.readLoop(ctx, conn, client, claims)
}

func (h *WSHandler) writeLoop(ctx context.Context, conn *websocket.Conn, client *hub.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-client.Outbox():
			if !ok {
				return
			}
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := wsjson.Write(wctx, conn, msg)
			cancel()
			if err != nil {
				slog.Warn("write failed, closing connection", "client_id", client.ID, "err", err)
				return
			}
		}
	}
}

func (h *WSHandler) readLoop(ctx context.Context, conn *websocket.Conn, client *hub.Client, claims *auth.Claims) {
	for {
		var raw json.RawMessage
		if err := wsjson.Read(ctx, conn, &raw); err != nil {
			return // connection closed or errored; defers in ServeHTTP clean up
		}

		var req hub.Message
		if err := json.Unmarshal(raw, &req); err != nil {
			client.Send(hub.Message{Type: hub.TypeError, Error: "malformed message"})
			continue
		}

		switch req.Type {
		case hub.TypeSubscribe:
			h.handleSubscribe(ctx, client, claims, req)
		case hub.TypeUnsub:
			h.hub.UnsubscribeLocal(client, req.Channel)
		case hub.TypePong:
			client.Pong() // resets the liveness deadline Heartbeat checks against
		default:
			client.Send(hub.Message{Type: hub.TypeError, Error: "unknown message type"})
		}
	}
}

func (h *WSHandler) handleSubscribe(ctx context.Context, client *hub.Client, claims *auth.Claims, req hub.Message) {
	if err := channel.CanSubscribe(claims, req.Channel); err != nil {
		client.Send(hub.Message{Type: hub.TypeError, Channel: req.Channel, Error: err.Error()})
		return
	}

	// Ensure this instance is subscribed to the Redis channel before
	// registering the local client, so a message published in the gap
	// between these two steps can't be missed.
	if err := h.ps.Subscribe(ctx, req.Channel, func(msg hub.Message) {
		h.hub.DeliverLocal(req.Channel, msg)
	}); err != nil {
		client.Send(hub.Message{Type: hub.TypeError, Channel: req.Channel, Error: "subscribe failed"})
		return
	}

	h.hub.SubscribeLocal(client, req.Channel)
	client.Send(hub.Message{Type: hub.TypeAck, Channel: req.Channel, Timestamp: time.Now()})

	// Replay anything missed since the client's last known message ID.
	var lastSeenID string
	if s, ok := req.Payload.(string); ok {
		lastSeenID = s
	}
	if lastSeenID != "" && h.redis != nil {
		msgs, err := pubsub.ReplaySince(ctx, h.redis, req.Channel, lastSeenID)
		if err != nil {
			slog.Warn("replay lookup failed", "channel", req.Channel, "err", err)
		}
		for _, m := range msgs {
			client.Send(m)
		}
	}
}
