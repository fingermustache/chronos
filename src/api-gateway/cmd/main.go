package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fingermustache/chronos/api-gateway/internal/config"
	"github.com/fingermustache/chronos/api-gateway/internal/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()
	srv := server.New(cfg, logger)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("api gateway starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-serverErr:
		logger.Error("server failed to start", "error", err)
		os.Exit(1)
	case <-quit:
		logger.Info("shutdown signal received, draining connections...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("forced shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped cleanly")
}
