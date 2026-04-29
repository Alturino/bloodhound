package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
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
	"github.com/alturino/bloodhound/internal/idx"
	"github.com/alturino/bloodhound/internal/log"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
	"github.com/alturino/bloodhound/internal/telemetry"
)

var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the worker service",
	RunE:  runServe,
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()

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
	idxStore := state.NewIdxStore(database, logger)
	// stockbitStore := state.NewStockbitStore(database, logger)

	httpClient := httpclient.NewClient(cfg)
	idxClient := idx.NewClient(
		httpClient,
		&cfg.App.IDX,
		logger.With(slog.String("tag", "idx.Client")),
		telemetry.AppTelemetry.Tracer,
	)
	// if cfg.App.IDX.MockMode {
	// 	logger.Info("using mock IDX client")
	// 	idxClient = idx.NewMockClient(
	// 		logger.With(slog.String("tag", "idx.MockClient")),
	// 		telemetry.AppTelemetry.Tracer,
	// 	)
	// }

	// stockbitClient := stockbit.NewClient(
	// 	httpClient,
	// 	&cfg.App.Stockbit,
	// 	logger.With(slog.String("tag", "stockbit.Client")),
	// 	telemetry.AppTelemetry.Tracer,
	// )

	attachmentProcessor := idx.NewAttachmentProcessor(
		&cfg.MinIO,
		logger.With(slog.String("tag", "attachment.Processor")),
		telemetry.AppTelemetry.Tracer,
		idxClient,
		stg,
		idxStore,
	)
	attachmentPool := idx.NewAttachmentPool(
		ctx,
		&cfg.App.IDX.WorkerPool,
		logger.With(slog.String("tag", "attachment.Pool")),
		telemetry.AppTelemetry.Tracer,
		attachmentProcessor,
	)

	announcementProcessor := idx.NewAnnouncementProcessor(
		logger,
		telemetry.AppTelemetry.Tracer,
		idxClient,
		idxStore,
		attachmentPool,
	)
	announcementPool := idx.NewAnnouncementPool(
		ctx,
		cfg.App.IDX.WorkerPool,
		logger,
		telemetry.AppTelemetry.Tracer,
		announcementProcessor,
	)

	idxWorker := idx.NewWorkerIdx(
		cfg,
		logger.With(slog.String("tag", "idx.Worker")),
		telemetry.AppTelemetry.Tracer,
		idxStore,
		idxClient,
		stg,
		announcementPool,
	)

	// stockbitconfig := &stockbit.Config{
	// 	Config:        cfg,
	// 	Logger:        logger,
	// 	Tracer:        telemetry.AppTelemetry.Tracer,
	// 	StockbitStore: stockbitStore,
	// }
	// stockbitWorker := stockbit.NewWorkerStockbit(stockbitconfig, stockbitClient)

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

	if err := idxWorker.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return err
	}

	// if err := stockbitWorker.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
	// 	logger.ErrorContext(ctx, "stockbit worker error", slog.Any("error", err))
	// 	return err
	// }
	return nil
}
