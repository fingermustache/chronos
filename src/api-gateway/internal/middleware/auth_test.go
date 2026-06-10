package middleware_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fingermustache/chronos/api-gateway/internal/middleware"
)

func authHandler(t *testing.T, logger *slog.Logger) http.Handler {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return middleware.Auth(logger)(next)
}

func TestAuth_MissingHeaderReturns401(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	w := httptest.NewRecorder()

	authHandler(t, logger).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuth_MalformedHeaderReturns401(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name   string
		header string
	}{
		{"no bearer prefix", "just-a-token"},
		{"wrong scheme", "Basic dXNlcjpwYXNz"},
		{"bearer with no token", "Bearer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()

			authHandler(t, logger).ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", w.Code)
			}
		})
	}
}

func TestAuth_ValidBearerTokenPasses(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	req.Header.Set("Authorization", "Bearer some.valid.token")
	w := httptest.NewRecorder()

	authHandler(t, logger).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
