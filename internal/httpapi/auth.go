package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

// RequireToken returns middleware that requires "Authorization: Bearer <token>"
// where the expected token is read from tokenFn on each request. When tokenFn is
// nil or returns "", auth is disabled (pass-through), matching the project's
// "blank = feature off" convention. Reading per request makes the token live.
func RequireToken(tokenFn func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ""
			if tokenFn != nil {
				token = tokenFn()
			}
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, bearerPrefix) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			presented := strings.TrimPrefix(header, bearerPrefix)
			if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
