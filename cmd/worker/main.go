package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fsnotify/fsnotify"
	_ "github.com/lib/pq"
	"github.com/spf13/viper"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/db"
	"github.com/alturino/bloodhound/internal/httpclient"
	"github.com/alturino/bloodhound/internal/idx"
	"github.com/alturino/bloodhound/internal/log"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/stockbit"
	"github.com/alturino/bloodhound/internal/storage"
	"github.com/alturino/bloodhound/internal/telemetry"
	"github.com/alturino/bloodhound/internal/worker"
)

func main() {
	// Load configuration
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load("bloodhound.yaml")
	if err != nil {
		err = fmt.Errorf("load config: %w", err)
		slog.ErrorContext(ctx, err.Error())
		return
	}

	logger := log.Get(&cfg.App)

	tmt, err := telemetry.New(ctx, cfg)
	if err != nil {
		err = fmt.Errorf("initialize telemetry: %w", err)
		logger.ErrorContext(ctx, err.Error())
		return
	}
	defer func() {
		if err := tmt.Shutdown(ctx); err != nil {
			err = fmt.Errorf("shutdown telemetry: %w", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
	}()

	// Initialize components
	stg, err := storage.NewMinIO(&cfg.MinIO, logger, telemetry.AppTelemetry.Tracer)
	if err != nil {
		err = fmt.Errorf("initialize storage: %w", err)
		logger.ErrorContext(ctx, err.Error())
		return
	}

	db, err := db.Get(ctx, &cfg.Database)
	if err != nil {
		err = fmt.Errorf("initialize database %w", err)
		logger.ErrorContext(ctx, err.Error())
		return
	}
	defer func() {
		if err := db.Close(); err != nil {
			err = fmt.Errorf("close database: %w", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
	}()
	stateStore := state.NewDBStore(db, logger)

	httpClient := httpclient.NewClient(cfg)
	idxClient := idx.NewClient(
		httpClient.Clone(),
		&cfg.App.IDX,
		logger.With(slog.String("tag", "idx.Client")),
		telemetry.AppTelemetry.Tracer,
	)
	if cfg.App.IDX.MockMode {
		logger.Info("using mock IDX client")
		idxClient = idx.NewMockClient(
			logger.With(slog.String("tag", "idx.MockClient")),
			telemetry.AppTelemetry.Tracer,
		)
	}

	stockbitClient := stockbit.NewClient(
		httpClient,
		&cfg.App.Stockbit,
		logger.With(slog.String("tag", "stockbit.Client")),
		telemetry.AppTelemetry.Tracer,
	)

	// Create and start worker
	w := worker.NewWorker(
		idxClient,
		stockbitClient,
		stg,
		stateStore,
		cfg,
		logger,
		telemetry.AppTelemetry.Tracer,
	)

	viper.OnConfigChange(func(in fsnotify.Event) {
		if !in.Has(fsnotify.Write) {
			return
		}
		if err := viper.MergeInConfig(); err != nil {
			err = fmt.Errorf("merge config file: %w", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
		if err := viper.Unmarshal(&cfg); err != nil {
			err = fmt.Errorf("unmarshal config: %w", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
		cfg.App.LogLevelVar.Set(cfg.App.LogLevel)
		logger = log.Get(&cfg.App)
		tmt, _ = telemetry.New(ctx, cfg)
		httpClient = httpclient.NewClient(cfg)
		idxClient = idx.NewClient(httpClient, &cfg.App.IDX, logger, telemetry.AppTelemetry.Tracer)
	})

	if err := w.Start(ctx); err != nil && errors.Is(err, context.Canceled) {
		logger.ErrorContext(ctx, err.Error())
		return
	}
}
