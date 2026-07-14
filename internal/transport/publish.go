package transport

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/realtime-notify/internal/hub"
)

// PublishHandler is the server-to-server entry point. Your existing
// backend (e.g. the SaaS starter kit's billing or org service) calls
// this whenever something happens that a connected client should know
// about -- "invoice paid", "teammate joined", "job finished". It's
// guarded by middleware.RequirePublishKey, not user JWTs, since the
// caller here is a trusted backend service, not an end user.
type PublishHandler struct {
	h *hub.Hub
}

func NewPublishHandler(h *hub.Hub) *PublishHandler {
	return &PublishHandler{h: h}
}

type publishRequest struct {
	Channel string `json:"channel"`
	Payload any    `json:"payload"`
}

func (p *PublishHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req publishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if req.Channel == "" {
		http.Error(w, "channel is required", http.StatusBadRequest)
		return
	}

	msg := hub.Message{
		ID:        uuid.NewString(),
		Type:      hub.TypeEvent,
		Channel:   req.Channel,
		Payload:   req.Payload,
		Timestamp: time.Now(),
	}

	if err := p.h.Publish(r.Context(), req.Channel, msg); err != nil {
		http.Error(w, "publish failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "published", "id": msg.ID})
}
