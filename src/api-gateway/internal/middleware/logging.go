package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// responseRecorder wraps ResponseWriter to capture the status code after the
// handler writes it — the default ResponseWriter doesn't expose this.
type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
	rr.ResponseWriter.WriteHeader(code)
}

// Logger logs method, path, status, and duration for every request.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

			defer func() {
				logger.Info("request",
					"id", GetRequestID(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"status", rec.status,
					"duration_ms", time.Since(start).Milliseconds(),
				)
			}()

			next.ServeHTTP(rec, r)
		})
	}
}
