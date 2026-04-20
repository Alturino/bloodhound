package cmd

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
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/db"
	"github.com/alturino/bloodhound/internal/httpclient"
	"github.com/alturino/bloodhound/internal/log"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
	"github.com/alturino/bloodhound/internal/telemetry"
	"github.com/alturino/bloodhound/internal/worker"
	"github.com/alturino/bloodhound/pkg/idx"
	"github.com/alturino/bloodhound/pkg/stockbit"
)

var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the worker service",
	RunE:  runServe,
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	configPath := viper.GetString("config")
	if configPath == "" {
		configPath = "bloodhound.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		err = fmt.Errorf("load config: %w", err)
		slog.ErrorContext(ctx, err.Error())
		return err
	}

	logger := log.Get(&cfg.App)
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic", slog.Any("panic", r))
			return
		}
	}()

	tmt, err := telemetry.New(ctx, cfg)
	if err != nil {
		err = fmt.Errorf("initialize telemetry: %w", err)
		logger.ErrorContext(ctx, err.Error())
		return err
	}
	defer func() {
		if err := tmt.Shutdown(ctx); err != nil {
			err = fmt.Errorf("shutdown telemetry: %w", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
	}()

	stg, err := storage.NewMinIO(&cfg.MinIO, logger, telemetry.AppTelemetry.Tracer)
	if err != nil {
		err = fmt.Errorf("initialize storage: %w", err)
		logger.ErrorContext(ctx, err.Error())
		return err
	}

	database, err := db.Get(ctx, &cfg.Database)
	if err != nil {
		err = fmt.Errorf("initialize database %w", err)
		logger.ErrorContext(ctx, err.Error())
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			err = fmt.Errorf("close database: %w", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
	}()
	stateStore := state.NewDBStore(database, logger)

	httpClient := httpclient.NewClient(cfg)
	idxClient := idx.NewClient(
		httpClient,
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

	workerCfg := &worker.WorkerConfig{
		Config:     cfg,
		Logger:     logger,
		Tracer:     telemetry.AppTelemetry.Tracer,
		StateStore: stateStore,
	}

	idxWorker := worker.NewWorkerIdx(workerCfg, idxClient, stg)
	stockbitWorker := worker.NewWorkerStockbit(workerCfg, stockbitClient)

	viper.OnConfigChange(func(in fsnotify.Event) {
		if !in.Has(fsnotify.Write) {
			return
		}
		if err := viper.MergeInConfig(); err != nil {
			err = fmt.Errorf("merge config file: %w", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
		if err := viper.Unmarshal(cfg); err != nil {
			err = fmt.Errorf("unmarshal config: %w", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
		cfg.App.LogLevelVar.Set(cfg.App.LogLevel)
	})

	go func() {
		if err := idxWorker.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(ctx, "idx worker error", slog.Any("error", err))
		}
	}()

	if err := stockbitWorker.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.ErrorContext(ctx, "stockbit worker error", slog.Any("error", err))
		return err
	}
	return nil
}