package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type healthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// Health handles GET /health.
// Returns 200 as long as the process is alive.
// A future version can add a DB ping here to represent a deeper health check.
func Health(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(healthResponse{
			Status:    "ok",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			logger.Error("failed to encode health response", "error", err)
		}
	}
}
