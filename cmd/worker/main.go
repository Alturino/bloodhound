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
	"github.com/alturino/bloodhound/internal/stockbit"
	"github.com/alturino/bloodhound/internal/storage"
	"github.com/alturino/bloodhound/internal/worker"
	_ "github.com/lib/pq"
	"database/sql"
)

func main() {
	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.App.LogLevel}))
	slog.SetDefault(logger)

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

	db, err := sql.Open("postgres", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}

	stateStore := state.NewDBStore(db)

	idxClient := http.NewClient(
		cfg.App.IDX.BaseURL,
		cfg.App.IDX.PageSize,
		logger,
		otel.Tracer("bloodhound-idx"),
	)

	stockbitClient := stockbit.NewClient(
		&cfg.App.Stockbit,
		os.Getenv("IDX_STOCKBIT_TOKEN"), // or from cfg if mapped
		logger,
		otel.Tracer("bloodhound-stockbit"),
	)

	// Create and start worker
	w := worker.NewWorker(
		idxClient,
		stockbitClient,
		stg,
		stateStore,
		&cfg,
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
