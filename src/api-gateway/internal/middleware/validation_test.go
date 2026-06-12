package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fingermustache/chronos/api-gateway/internal/middleware"
)

func TestValidation_AllowsGetWithoutContentType(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	w := httptest.NewRecorder()

	middleware.Validation(next).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestValidation_AllowsPostWithJSONContentType(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	middleware.Validation(next).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestValidation_AllowsPostWithJSONContentTypeAndCharset(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	w := httptest.NewRecorder()

	middleware.Validation(next).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestValidation_BlocksMutationWithoutContentType(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/tasks", nil)
			w := httptest.NewRecorder()

			middleware.Validation(next).ServeHTTP(w, req)

			if w.Code != http.StatusUnsupportedMediaType {
				t.Errorf("expected 415, got %d", w.Code)
			}
		})
	}
}

func TestValidation_BlocksMutationWithWrongContentType(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	middleware.Validation(next).ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", w.Code)
	}
}
