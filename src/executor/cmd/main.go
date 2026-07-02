package main

import (
	"log/slog"
	"os"

	"github.com/fingermustache/chronos/executor/internal/config"
	"github.com/fingermustache/chronos/executor/internal/execution"
	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/database"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()

	db, err := database.New(cfg.Database)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	brokerConn, err := broker.NewConnection(cfg.Broker)
	if err != nil {
		logger.Error("failed to connect to broker", "error", err)
		os.Exit(1)
	}
	defer brokerConn.Close()

	if err := broker.SetupTopology(brokerConn); err != nil {
		logger.Error("failed to setup broker topology", "error", err)
		os.Exit(1)
	}

	consumer, err := broker.NewConsumer(brokerConn)
	if err != nil {
		logger.Error("failed to create consumer", "error", err)
		os.Exit(1)
	}

	exec := execution.New(consumer, logger)

	logger.Info("executor starting")
	if err := exec.Run(); err != nil {
		logger.Error("executor exited with error", "error", err)
		os.Exit(1)
	}
}
