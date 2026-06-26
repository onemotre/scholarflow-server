package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

// RequireToken returns a chi-compatible middleware that requires
// "Authorization: Bearer <token>" on the wrapped routes. When token is empty
// the middleware is a pass-through (auth disabled), matching the project's
// "blank = feature off" convention.
func RequireToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
