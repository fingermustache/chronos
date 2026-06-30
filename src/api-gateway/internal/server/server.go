package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fingermustache/chronos/api-gateway/internal/config"
	"github.com/fingermustache/chronos/api-gateway/internal/handler"
	"github.com/fingermustache/chronos/api-gateway/internal/middleware"
	"github.com/fingermustache/chronos/api-gateway/internal/service"
)

// New wires up the router, middleware stack, and routes, then returns
// a configured http.Server ready to be started by main.
func New(cfg config.Config, logger *slog.Logger, taskSvc service.TaskService) *http.Server {
	r := chi.NewRouter()

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRPM)

	// Global middleware — runs on every request regardless of route
	r.Use(middleware.Recovery(logger)) // outermost — catches panics everywhere
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger(logger))
	r.Use(middleware.CORS)
	r.Use(rateLimiter.Middleware(logger))
	r.Use(middleware.Validation)

	// Public routes — no auth required
	r.Group(func(r chi.Router) {
		r.Get("/health", handler.Health(logger))
	})

	// Protected routes — auth required
	// Auth is scoped here only, not in the global stack above
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(logger))

		tasks := handler.NewTaskHandler(taskSvc, logger)
		r.Post("/tasks", tasks.Create)
		r.Get("/tasks", tasks.List)
		r.Get("/tasks/{id}", tasks.GetByID)
		r.Put("/tasks/{id}", tasks.Update)
		r.Delete("/tasks/{id}", tasks.Delete)
	})

	return &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}
