package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
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
				// Reset window — this also reclaims memory for IPs that previously
				// hit the limit and have since gone quiet.
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

// extractIP returns the real client IP, respecting common proxy headers.
// TODO: add a background goroutine to evict entries older than 2*time.Minute
// to prevent unbounded map growth under a high volume of unique client IPs.
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if parts := strings.SplitN(xff, ",", 2); len(parts) > 0 {
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
