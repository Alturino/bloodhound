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

var IDXRunAnnCmd = &cobra.Command{
	Use:     "announcements",
	Aliases: []string{"ann", "a"},
	Short:   "Run idx announcements worker only",
	RunE:    idxWorkerAnn,
}

func idxWorkerAnn(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	StartPprofServer(ctx)

	configPath := ResolveConfigPath()
	cfg, ctx, err := LoadConfig(ctx, configPath)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "loaded config")

	logger, err := log.Get(cfg.App)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	defer RecoverPanic(logger)

	deps, err := initIDXDeps(ctx, cfg)
	if err != nil {
		return fmt.Errorf("init idx deps: %w", err)
	}
	defer ShutdownTelemetry(ctx, deps.tmt, logger)
	defer CloseDatabase(ctx, deps.db, logger)

	announcementPool := idx.NewAnnouncementPool(
		ctx,
		cfg.App.IDX.WorkerPool.AnnouncementWorkers,
		deps.logger.With(slog.String("tag", "idx.announcementPool")),
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
		deps.logger.With(slog.String("tag", "idx.AnnouncementScheduler")),
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
