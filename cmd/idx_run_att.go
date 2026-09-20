package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/alturino/bloodhound/internal/idx"
	"github.com/alturino/bloodhound/internal/log"
)

var IDXRunAttCmd = &cobra.Command{
	Use:     "attachments",
	Aliases: []string{"att"},
	Short:   "Run idx attachments downloader only",
	RunE:    idxWorkerAtt,
}

func idxWorkerAtt(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	StartPprofServer(ctx)

	configPath := ResolveConfigPath()
	cfg, ctx, err := LoadConfig(ctx, configPath)
	if err != nil {
		return err
	}

	logger, err := log.Get(cfg.App)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	defer RecoverPanic(logger)

	logger.DebugContext(ctx, "initializing idx dependencies")
	deps, err := initIDXDeps(ctx, cfg)
	if err != nil {
		return fmt.Errorf("init idx deps: %w", err)
	}
	defer ShutdownTelemetry(ctx, deps.tmt, logger)
	defer CloseDatabase(ctx, deps.db, logger)
	logger.InfoContext(ctx, "initialized idx dependencies")

	logger.DebugContext(ctx, "initializing attachment worker")
	attachmentWorker := idx.NewAttachmentWorker(
		cfg.Storage.MinIO,
		deps.logger.With(slog.String("tag", "idx.AttachmentWorker")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.client,
		deps.stg,
	)
	logger.InfoContext(ctx, "initialized attachment worker")

	logger.DebugContext(ctx, "initializing attachment pool")
	attachmentPool := idx.NewAttachmentPool(
		ctx,
		cfg.App.IDX.WorkerPool,
		deps.logger.With(slog.String("tag", "idx.attachmentPool")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		attachmentWorker,
		deps.attStore,
	)
	defer attachmentPool.Shutdown()
	logger.InfoContext(ctx, "initialized attachment pool")

	logger.DebugContext(ctx, "initializing attachment scheduler")
	attachmentScheduler := idx.NewAttachmentScheduler(
		ctx,
		cfg.App.IDX.Scheduler,
		deps.logger.With(slog.String("tag", "idx.AttachmentScheduler")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.attStore,
		attachmentPool,
	)
	attachmentScheduler.Start()
	defer attachmentScheduler.Shutdown()
	logger.InfoContext(ctx, "initialized attachment scheduler")

	WatchConfigChange(ctx, cfg, logger)

	<-ctx.Done()
	logger.InfoContext(ctx, "context done, stopping")

	return nil
}
