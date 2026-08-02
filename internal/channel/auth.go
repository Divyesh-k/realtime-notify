package channel

import (
	"fmt"
	"strings"

	"github.com/yourname/realtime-notify/internal/auth"
)

// Channel naming convention, enforced (not just documented) by CanSubscribe:
//
//	user:<user_id>   — private to that user only
//	org:<org_id>     — any member of that org
//	broadcast:<name> — public, anyone with a valid token
//
// A connected client can open exactly one WebSocket and still only
// receive the channels they're entitled to, because authorization is
// checked on every subscribe call, not just once at connect time. This
// is the difference between "logged in" and "allowed to see this data."

var ErrForbidden = fmt.Errorf("not authorized to subscribe to this channel")

func CanSubscribe(claims *auth.Claims, channel string) error {
	switch {
	case strings.HasPrefix(channel, "user:"):
		if strings.TrimPrefix(channel, "user:") != claims.UserID {
			return ErrForbidden
		}
	case strings.HasPrefix(channel, "org:"):
		if strings.TrimPrefix(channel, "org:") != claims.OrgID {
			return ErrForbidden
		}
	case strings.HasPrefix(channel, "broadcast:"):
		// any authenticated client may subscribe
	default:
		return fmt.Errorf("unknown channel namespace: %s", channel)
	}
	return nil
}
