package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/yourname/realtime-notify/internal/auth"
)

type ctxKey string

const claimsCtxKey ctxKey = "claims"

// RequireAuth protects plain HTTP endpoints (e.g. the server-to-server
// publish endpoint uses a separate API key check, but any future
// user-facing REST route goes through this). WS/SSE handlers verify the
// token themselves from a query param since browsers can't set custom
// headers on a WebSocket upgrade request.
func RequireAuth(v *auth.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token := strings.TrimPrefix(header, "Bearer ")
			if token == "" || token == header {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			claims, err := v.Verify(token)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), claimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ClaimsFromContext(ctx context.Context) (*auth.Claims, bool) {
	c, ok := ctx.Value(claimsCtxKey).(*auth.Claims)
	return c, ok
}
