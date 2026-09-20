package cmd

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

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

	StartPprofServer(ctx)

	configPath := ResolveConfigPath()
	cfg, ctx, err := LoadConfig(ctx, configPath)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "loaded config")

	deps, err := initIDXDeps(ctx, cfg)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "initialized idx dependencies")

	defer RecoverPanic(deps.logger)
	defer ShutdownTelemetry(ctx, deps.tmt, deps.logger)
	defer CloseDatabase(ctx, deps.db, deps.logger)

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
		ctx,
		cfg.App.IDX.WorkerPool,
		logger.With(slog.String("tag", "idx.attachmentPool")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		attachmentWorker,
		deps.attStore,
	)
	attachmentScheduler := idx.NewAttachmentScheduler(
		ctx,
		cfg.App.IDX.Scheduler,
		logger.With(slog.String("tag", "attachment.Scheduler")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.attStore,
		attachmentPool,
	)
	defer attachmentScheduler.Shutdown()

	announcementPool := idx.NewAnnouncementPool(
		ctx,
		cfg.App.IDX.WorkerPool.AnnouncementWorkers,
		logger.With(slog.String("tag", "idx.announcementPool")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.annStore,
		deps.attStore,
	)
	announcementScheduler := idx.NewAnnouncementScheduler(
		ctx,
		cfg.App.IDX.Scheduler,
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
	announcementScheduler.Start()
	defer announcementScheduler.Shutdown()

	WatchConfigChange(ctx, cfg, logger)

	<-ctx.Done()
	logger.InfoContext(ctx, "context done, stopping")

	return nil
}
