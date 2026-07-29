package middleware

import "net/http"

// RequirePublishKey guards the server-to-server publish endpoint. Your
// backend (e.g. the SaaS starter kit) calls POST /api/v1/publish with
// this key whenever it wants to push an event -- "user123 got a new
// invoice", "order #456 shipped" -- to whoever's listening. This is
// deliberately a static shared secret, not a JWT: it's service-to-service,
// not user-facing.
func RequirePublishKey(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-Publish-Key")
			if key == "" || key != expected {
				http.Error(w, "invalid publish key", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
