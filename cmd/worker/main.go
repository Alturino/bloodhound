package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/otel"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/http"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
	"github.com/alturino/bloodhound/internal/worker"
)

func main() {
	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize components
	stg, err := storage.NewMinIOStorage(
		cfg.MinIO.Endpoint,
		cfg.MinIO.AccessKey,
		cfg.MinIO.SecretKey,
		cfg.MinIO.UseSSL,
		logger,
	)
	if err != nil {
		log.Fatalf("failed to initialize storage: %v", err)
	}

	stateStore, err := state.NewFileStore("data/state.json")
	if err != nil {
		log.Fatalf("failed to initialize state store: %v", err)
	}

	idxClient := http.NewClient(
		cfg.IDX.BaseURL,
		cfg.IDX.PageSize,
		logger,
		otel.Tracer("bloodhound-idx"),
	)

	// Create and start worker
	w := worker.NewWorker(
		idxClient,
		stg,
		stateStore,
		cfg.MinIO.Bucket,
		cfg.Scheduler.Interval,
		logger,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := w.Start(ctx); err != nil && err != context.Canceled {
		log.Fatalf("worker failed: %v", err)
	}
}
