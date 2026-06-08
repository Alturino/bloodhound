package cmd

import (
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/blobstorage"
	"github.com/alturino/bloodhound/internal/db"
	"github.com/alturino/bloodhound/internal/httpclient"
	"github.com/alturino/bloodhound/internal/idx"
	"github.com/alturino/bloodhound/internal/log"
	"github.com/alturino/bloodhound/internal/telemetry"
)

var IDXWorker = &cobra.Command{
	Use:     "idx run",
	Short:   "Run the worker service",
	RunE:    idxWorker,
	Aliases: []string{"ir"},
}

func idxWorker(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := http.ListenAndServe(":9999", nil); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return
		}
	}()

	configPath := viper.GetString("config")
	if configPath == "" {
		configPath = "bloodhound.yaml"
	}
	ctx = slogctx.Append(ctx, slog.String("config_path", configPath))

	slog.InfoContext(ctx, "load config")
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

	logger.DebugContext(ctx, "initialize blobstorage")
	stg, err := blobstorage.NewStorage(&cfg.Storage, logger, telemetry.AppTelemetry.Tracer)
	if err != nil {
		err = fmt.Errorf("initialize storage: %w", err)
		logger.ErrorContext(ctx, err.Error())
		return err
	}
	logger.InfoContext(ctx, "initialized blobstorage")

	database, err := db.Get(ctx, &cfg.Database)
	if err != nil {
		err = fmt.Errorf("initialize database: %w", err)
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
	announcementStore := idx.NewAnnouncementStore(database, logger, telemetry.AppTelemetry.Tracer)
	attachmentStore := idx.NewAttachmentStore(database, logger, telemetry.AppTelemetry.Tracer)

	httpClient := httpclient.NewClient(cfg, telemetry.AppTelemetry.Tracer)
	idxClient := idx.NewClient(
		httpClient,
		&cfg.App.IDX,
		logger.With(slog.String("tag", "idx.Client")),
		telemetry.AppTelemetry.Tracer,
	)

	attachmentWorker := idx.NewAttachmentWorker(
		&cfg.Storage.MinIO,
		logger.With(slog.String("tag", "idx.Processor")),
		telemetry.AppTelemetry.Tracer,
		telemetry.AppTelemetry.Metrics,
		idxClient,
		stg,
	)
	attachmentPool := idx.NewAttachmentPool(
		ctx,
		database,
		&cfg.App.IDX.WorkerPool,
		logger.With(slog.String("tag", "attachment.Pool")),
		telemetry.AppTelemetry.Tracer,
		telemetry.AppTelemetry.Metrics,
		attachmentWorker,
		attachmentStore,
	)
	defer attachmentPool.Shutdown()

	idxWorker := idx.NewWorkerIdx(
		ctx,
		cfg,
		logger.With(slog.String("tag", "idx.Worker")),
		telemetry.AppTelemetry.Tracer,
		telemetry.AppTelemetry.Metrics,
		announcementStore,
		attachmentStore,
		idxClient,
		database,
		stg,
	)
	defer idxWorker.Shutdown()

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

	<-ctx.Done()
	logger.InfoContext(ctx, "received context done, stopping")

	return nil
}
