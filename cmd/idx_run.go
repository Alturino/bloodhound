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
	"github.com/alturino/bloodhound/internal/idx"
)

var IDXRunCmd = &cobra.Command{
	Use:     "run",
	Short:   "Run both idx announcements and attachments workers",
	RunE:    idxWorkerRunAll,
	Aliases: []string{"r"},
}

func idxWorkerRunAll(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("load config: %w", err)
	}

	deps, err := initIDXDeps(ctx, cfg)
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			deps.logger.Error("panic", slog.Any("panic", r))
		}
	}()

	defer func() {
		if err := deps.tmt.Shutdown(ctx); err != nil {
			deps.logger.ErrorContext(ctx, fmt.Errorf("shutdown telemetry: %w", err).Error())
		}
	}()

	defer func() {
		if err := deps.db.Close(); err != nil {
			deps.logger.ErrorContext(ctx, fmt.Errorf("close database: %w", err).Error())
		}
	}()

	logger := deps.logger

	attachmentWorker := idx.NewAttachmentWorker(
		cfg.Storage.MinIO,
		logger.With(slog.String("tag", "idx.Processor")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.client,
		deps.stg,
	)
	attachmentPool := idx.NewAttachmentPool(
		cfg.App.IDX.WorkerPool,
		logger.With(slog.String("tag", "idx.attachmentPool")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		attachmentWorker,
		deps.attStore,
	)
	attachmentScheduler := idx.NewAttachmentScheduler(
		ctx,
		cfg.Scheduler,
		logger.With(slog.String("tag", "attachment.Scheduler")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.attStore,
		attachmentPool,
	)
	attachmentScheduler.Start()
	defer attachmentScheduler.Shutdown()

	announcementPool := idx.NewAnnouncementPool(
		cfg.App.IDX.WorkerPool.AnnouncementWorkers,
		logger.With(slog.String("tag", "idx.announcementPool")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.annStore,
		deps.attStore,
	)
	announcementScheduler := idx.NewAnnouncementScheduler(
		ctx,
		cfg.Scheduler,
		cfg.App.IDX.PageSize,
		cfg.App.IDX.WorkerPool.AnnouncementWorkers,
		logger.With(slog.String("tag", "idx.AnnouncementScheduler")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.annStore,
		deps.attStore,
		deps.client,
		announcementPool,
	)
	defer announcementScheduler.Shutdown()

	viper.OnConfigChange(func(in fsnotify.Event) {
		if !in.Has(fsnotify.Write) {
			return
		}
		if err := viper.MergeInConfig(); err != nil {
			deps.logger.ErrorContext(ctx, fmt.Errorf("merge config file: %w", err).Error())
			return
		}
		if err := viper.Unmarshal(cfg); err != nil {
			deps.logger.ErrorContext(ctx, fmt.Errorf("unmarshal config: %w", err).Error())
			return
		}
		cfg.App.LogLevelVar.Set(cfg.App.LogLevel)
	})

	<-ctx.Done()
	logger.InfoContext(ctx, "context done, stopping")

	return nil
}
