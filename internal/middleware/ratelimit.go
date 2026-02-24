package middleware

import (
	"net/http"
	"sync"
	"time"
)

type visitor struct {
	count    int
	lastSeen time.Time
}

// RateLimiter is a simple in-memory per-IP rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a rate limiter that allows `limit` requests per `window` per IP.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		window:   window,
	}

	// Cleanup stale entries every minute
	go func() {
		for {
			time.Sleep(time.Minute)
			rl.mu.Lock()
			for ip, v := range rl.visitors {
				if time.Since(v.lastSeen) > rl.window*2 {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()

	return rl
}

// Middleware returns an HTTP middleware that enforces rate limits.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr

		rl.mu.Lock()
		v, exists := rl.visitors[ip]
		if !exists || time.Since(v.lastSeen) > rl.window {
			rl.visitors[ip] = &visitor{count: 1, lastSeen: time.Now()}
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
			return
		}

		v.count++
		v.lastSeen = time.Now()
		if v.count > rl.limit {
			rl.mu.Unlock()
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
