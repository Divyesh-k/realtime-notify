package hub

import "time"

// Message is the wire format for everything flowing through the system —
// client control frames and server-pushed events alike. Payload is left
// as `any` (not json.RawMessage) so publish.go can hand it an arbitrary
// application payload and json.Marshal handles encoding once, at the
// pubsub boundary, instead of double-encoding.
type Message struct {
	ID        string      `json:"id,omitempty"`
	Type      MessageType `json:"type"`
	Channel   string      `json:"channel,omitempty"`
	Payload   any         `json:"payload,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp time.Time   `json:"ts,omitempty"`
}

type MessageType string

const (
	// Client -> server
	TypeSubscribe MessageType = "subscribe"
	TypeUnsub     MessageType = "unsubscribe"
	TypePong      MessageType = "pong"

	// Server -> client
	TypeEvent MessageType = "event"
	TypeAck   MessageType = "ack"
	TypeError MessageType = "error"
	TypePing  MessageType = "ping"
)
