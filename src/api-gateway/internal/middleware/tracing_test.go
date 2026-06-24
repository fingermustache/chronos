//go:build !integration

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fingermustache/chronos/api-gateway/internal/middleware"
)

func TestRequestID_GeneratesIDWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Handler that checks the context was populated
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := middleware.GetRequestID(r.Context())
		if id == "" || id == "unknown" {
			t.Error("expected a request ID in context, got none")
		}
	})

	middleware.RequestID(next).ServeHTTP(w, req)

	// ID should also be echoed in the response header
	if w.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID response header to be set")
	}
}

func TestRequestID_RespectsIncomingHeader(t *testing.T) {
	const existingID = "my-upstream-id"

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existingID)
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := middleware.GetRequestID(r.Context())
		if id != existingID {
			t.Errorf("expected request ID %q, got %q", existingID, id)
		}
	})

	middleware.RequestID(next).ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != existingID {
		t.Errorf("expected response header %q, got %q", existingID, got)
	}
}
