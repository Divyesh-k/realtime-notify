package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signToken(t *testing.T, secret string, claims Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return s
}

func TestVerifyValidToken(t *testing.T) {
	v := NewVerifier("test-secret")
	claims := Claims{
		UserID: "u1",
		OrgID:  "o1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := signToken(t, "test-secret", claims)

	got, err := v.Verify(token)
	if err != nil {
		t.Fatalf("expected valid token to verify, got %v", err)
	}
	if got.UserID != "u1" || got.OrgID != "o1" {
		t.Fatalf("unexpected claims: %+v", got)
	}
}

func TestVerifyExpiredToken(t *testing.T) {
	v := NewVerifier("test-secret")
	claims := Claims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	token := signToken(t, "test-secret", claims)

	if _, err := v.Verify(token); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestVerifyWrongSecretRejected(t *testing.T) {
	v := NewVerifier("real-secret")
	claims := Claims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := signToken(t, "wrong-secret", claims)

	if _, err := v.Verify(token); err == nil {
		t.Fatal("expected token signed with wrong secret to be rejected")
	}
}

func TestVerifyMissingUserIDRejected(t *testing.T) {
	v := NewVerifier("test-secret")
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := signToken(t, "test-secret", claims)

	if _, err := v.Verify(token); err == nil {
		t.Fatal("expected token with no user id (sub claim) to be rejected")
	}
}

func TestVerifyRejectsUnexpectedSigningMethod(t *testing.T) {
	v := NewVerifier("test-secret")
	// RS256 would require an *rsa.PrivateKey to sign; we only need to
	// prove HMAC-only tokens are accepted and non-HMAC tokens are
	// rejected before a signature is even checked, so build the token
	// manually and confirm Verify's algorithm guard fires by using a
	// clearly wrong "none" algorithm token constructed by hand.
	unsecured := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJzdWIiOiJ1MSJ9." // header.payload with alg=none, no signature
	if _, err := v.Verify(unsecured); err == nil {
		t.Fatal("expected a token with alg=none to be rejected")
	}
}

func TestIssueDevTokenRoundTrips(t *testing.T) {
	v := NewVerifier("test-secret")
	token, err := v.IssueDevToken("u1", "o1", time.Hour)
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	claims, err := v.Verify(token)
	if err != nil {
		t.Fatalf("expected issued dev token to verify, got %v", err)
	}
	if claims.UserID != "u1" || claims.OrgID != "o1" {
		t.Fatalf("unexpected claims round-trip: %+v", claims)
	}
}
