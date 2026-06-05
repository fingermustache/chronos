package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

// Auth validates the Bearer token format on every request except /health.
// Phase 1: structural check only.
// Phase 2: replace the TODO with real JWT verification (e.g. golang-jwt/jwt).
func Auth(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			header := r.Header.Get("Authorization")
			if header == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, "authorization header must be: Bearer <token>", http.StatusUnauthorized)
				return
			}

			// TODO Phase 2: verify parts[1] as a signed JWT
			logger.Debug("auth stub: format valid, skipping signature verification",
				"id", GetRequestID(r.Context()),
			)

			next.ServeHTTP(w, r)
		})
	}
}
