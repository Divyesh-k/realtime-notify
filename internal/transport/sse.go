package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/yourname/realtime-notify/internal/auth"
	"github.com/yourname/realtime-notify/internal/channel"
	"github.com/yourname/realtime-notify/internal/hub"
	"github.com/yourname/realtime-notify/internal/pubsub"
)

// SSEHandler exists for the subset of environments — corporate proxies,
// some mobile carriers, older infra — that block WebSocket upgrades but
// allow plain long-lived HTTP. It's strictly one-way (server -> client);
// subscribing to a channel is done via a query param at connect time
// rather than a message frame, since SSE has no client-to-server leg.
type SSEHandler struct {
	hub      *hub.Hub
	ps       pubsub.PubSub
	verifier *auth.Verifier
}

func NewSSEHandler(h *hub.Hub, ps pubsub.PubSub, v *auth.Verifier) *SSEHandler {
	return &SSEHandler{hub: h, ps: ps, verifier: v}
}

func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	claims, err := h.verifier.Verify(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ch := r.URL.Query().Get("channel")
	if err := channel.CanSubscribe(claims, ch); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	client := hub.NewClient(claims.UserID, claims.OrgID)
	h.hub.Register(client)
	defer h.hub.Unregister(client)

	if err := h.ps.Subscribe(ctx, ch, func(msg hub.Message) {
		h.hub.DeliverLocal(ch, msg)
	}); err != nil {
		http.Error(w, "subscribe failed", http.StatusInternalServerError)
		return
	}
	h.hub.SubscribeLocal(client, ch)

	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case msg, ok := <-client.Outbox():
			if !ok {
				return
			}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "id: %s\ndata: %s\n\n", msg.ID, data)
			flusher.Flush()
		}
	}
}
