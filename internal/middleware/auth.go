package middleware

import (
	"net/http"
)

const apiKeyHeader = "X-Media-API-Key"

// APIKeyAuth validates requests against a shared API key.
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(apiKeyHeader)
			if key == "" {
				http.Error(w, `{"error":"missing API key"}`, http.StatusUnauthorized)
				return
			}
			if key != apiKey {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
