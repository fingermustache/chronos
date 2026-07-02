package main

import (
	"log/slog"
	"os"
	"time"
	_ "time/tzdata"

	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/scheduler/internal/config"
	schedulerrepo "github.com/fingermustache/chronos/scheduler/internal/repository"
	"github.com/fingermustache/chronos/scheduler/internal/scheduling"
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

	pub, err := broker.NewPublisher(brokerConn)
	if err != nil {
		logger.Error("failed to create publisher", "error", err)
		os.Exit(1)
	}
	defer pub.Close()

	schedCfg := scheduling.Config{
		PollInterval:   time.Duration(cfg.PollIntervalSeconds) * time.Second,
		ClaimBatchSize: cfg.ClaimBatchSize,
	}

	repo := schedulerrepo.NewTaskRepository(db)
	sched := scheduling.New(db, repo, pub, logger, schedCfg)

	logger.Info("scheduler starting")
	if err := sched.Run(); err != nil {
		logger.Error("scheduler exited with error", "error", err)
		os.Exit(1)
	}
}
