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

	slog.DebugContext(ctx, "loading config")
	cfg, err := config.Load(configPath)
	if err != nil {
		err = fmt.Errorf("load config: %v", err)
		return err
	}
	slog.InfoContext(ctx, "loaded config")

	logger, err := log.Get(cfg.App)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic", slog.Any("panic", r))
			return
		}
	}()

	logger.DebugContext(ctx, "initializing idx dependencies")
	deps, err := initIDXDeps(ctx, cfg)
	if err != nil {
		err = fmt.Errorf("init idx deps: %v", err)
		return err
	}
	defer func() {
		if err := deps.tmt.Shutdown(ctx); err != nil {
			err = fmt.Errorf("shutdown telemetry: %w", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
	}()
	defer func() {
		if err := deps.db.Close(); err != nil {
			err = fmt.Errorf("close database: %v", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
	}()
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

	viper.OnConfigChange(func(in fsnotify.Event) {
		if !in.Has(fsnotify.Write) {
			return
		}
		if err := viper.MergeInConfig(); err != nil {
			err = fmt.Errorf("merge config file: %v", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
		if err := viper.Unmarshal(cfg); err != nil {
			err = fmt.Errorf("unmarshal config: %v", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
		cfg.App.LogLevelVar.Set(cfg.App.LogLevel)
	})

	<-ctx.Done()
	logger.InfoContext(ctx, "context done, stopping")

	return nil
}
