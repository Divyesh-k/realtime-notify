package channel

import (
	"testing"

	"github.com/yourname/realtime-notify/internal/auth"
)

func claims(userID, orgID string) *auth.Claims {
	return &auth.Claims{UserID: userID, OrgID: orgID}
}

func TestCanSubscribeUserChannel(t *testing.T) {
	c := claims("u1", "o1")
	if err := CanSubscribe(c, "user:u1"); err != nil {
		t.Fatalf("expected own user channel to be allowed, got %v", err)
	}
	if err := CanSubscribe(c, "user:u2"); err == nil {
		t.Fatal("expected another user's channel to be forbidden")
	}
}

func TestCanSubscribeOrgChannel(t *testing.T) {
	c := claims("u1", "o1")
	if err := CanSubscribe(c, "org:o1"); err != nil {
		t.Fatalf("expected own org channel to be allowed, got %v", err)
	}
	if err := CanSubscribe(c, "org:o2"); err == nil {
		t.Fatal("expected another org's channel to be forbidden")
	}
}

func TestCanSubscribeBroadcastChannel(t *testing.T) {
	c := claims("u1", "o1")
	if err := CanSubscribe(c, "broadcast:announcements"); err != nil {
		t.Fatalf("expected broadcast channel to be allowed for any authenticated user, got %v", err)
	}
}

func TestCanSubscribeUnknownNamespaceRejected(t *testing.T) {
	c := claims("u1", "o1")
	if err := CanSubscribe(c, "admin:secrets"); err == nil {
		t.Fatal("expected unknown namespace to be rejected")
	}
}
