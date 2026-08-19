// Package auth verifies access tokens issued by the primary auth service
// (e.g. the SaaS backend's /auth/login). This service does not issue
// tokens itself — it only validates them.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
)

// Claims mirrors the access token shape issued elsewhere in the stack:
// user id + org id, so channel authorization can be scoped without a
// database round trip on every connection.
type Claims struct {
	UserID string `json:"sub"`
	OrgID  string `json:"org_id"`
	jwt.RegisteredClaims
}

type Verifier struct {
	secret []byte
}

func NewVerifier(secret string) *Verifier {
	return &Verifier{secret: []byte(secret)}
}

func (v *Verifier) Verify(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))

	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.UserID == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// IssueDevToken is a convenience for local testing only — it is never
// wired into a production route. Real tokens come from the auth service.
func (v *Verifier) IssueDevToken(userID, orgID string, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID: userID,
		OrgID:  orgID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(v.secret)
}
