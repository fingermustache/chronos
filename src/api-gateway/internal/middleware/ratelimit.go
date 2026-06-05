package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu          sync.Mutex
	counts      map[string]int
	windowStart map[string]time.Time
	rpm         int
}

func NewRateLimiter(rpm int) *RateLimiter {
	return &RateLimiter{
		counts:      make(map[string]int),
		windowStart: make(map[string]time.Time),
		rpm:         rpm,
	}
}

func (rl *RateLimiter) Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)

			rl.mu.Lock()
			now := time.Now()
			if start, ok := rl.windowStart[ip]; !ok || now.Sub(start) > time.Minute {
				rl.counts[ip] = 0
				rl.windowStart[ip] = now
			}
			rl.counts[ip]++
			count := rl.counts[ip]
			rl.mu.Unlock()

			if count > rl.rpm {
				logger.Warn("rate limit exceeded",
					"ip", ip,
					"id", GetRequestID(r.Context()),
				)
				w.Header().Set("Retry-After", "60")
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractIP strips the port from RemoteAddr so requests from the same
// IP on different ports are counted as the same client.
func extractIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr had no port (shouldn't happen, but fall back gracefully)
		return r.RemoteAddr
	}
	return ip
}
