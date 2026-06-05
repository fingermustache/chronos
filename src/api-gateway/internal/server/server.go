package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fingermustache/chronos/api-gateway/internal/config"
	"github.com/fingermustache/chronos/api-gateway/internal/handler"
	"github.com/fingermustache/chronos/api-gateway/internal/middleware"
)

// New wires up the router, middleware stack, and routes, then returns
// a configured http.Server ready to be started by main.
func New(cfg config.Config, logger *slog.Logger) *http.Server {
	r := chi.NewRouter()

	// Middleware runs top to bottom on every request
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRPM)

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger(logger))
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.CORS)
	r.Use(rateLimiter.Middleware(logger))
	r.Use(middleware.Auth(logger))

	// Routes
	r.Get("/health", handler.Health)

	return &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}
