package middleware

import (
	"net/http"
	"strings"
)

// Validation checks basic request structure before it reaches a handler.
// Phase 1: enforces Content-Type on mutation requests.
// Phase 2: add per-route JSON schema validation.
func Validation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "application/json") {
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
