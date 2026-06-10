package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fingermustache/chronos/api-gateway/internal/middleware"
)

func TestCORS_SetsHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	middleware.CORS(next).ServeHTTP(w, req)

	headers := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, PUT, PATCH, DELETE, OPTIONS",
	}

	for header, expected := range headers {
		if got := w.Header().Get(header); got != expected {
			t.Errorf("header %q: expected %q, got %q", header, expected, got)
		}
	}
}

func TestCORS_PreflightReturns204(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should never reach here for an OPTIONS request
		t.Error("handler should not be called for preflight request")
	})

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	w := httptest.NewRecorder()

	middleware.CORS(next).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS preflight, got %d", w.Code)
	}
}
