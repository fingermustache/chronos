//go:build !integration

package middleware_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fingermustache/chronos/api-gateway/internal/middleware"
)

func TestRateLimiter_AllowsRequestsUnderLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rl := middleware.NewRateLimiter(5) // 5 requests per minute

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := rl.Middleware(logger)(next)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}
}

func TestRateLimiter_BlocksRequestsOverLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rl := middleware.NewRateLimiter(2) // limit to 2 per minute

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := rl.Middleware(logger)(next)

	// Send 3 requests from the same IP — the third should be blocked
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:5555"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if i < 2 && w.Code != http.StatusOK {
			t.Errorf("request %d should pass, got %d", i+1, w.Code)
		}
		if i == 2 && w.Code != http.StatusTooManyRequests {
			t.Errorf("request %d should be blocked with 429, got %d", i+1, w.Code)
		}
	}
}

func TestRateLimiter_TracksIPsSeparately(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rl := middleware.NewRateLimiter(1) // 1 request per minute per IP

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := rl.Middleware(logger)(next)

	ips := []string{"1.1.1.1:0", "2.2.2.2:0"}

	for _, ip := range ips {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("first request from %s should pass, got %d", ip, w.Code)
		}
	}
}
