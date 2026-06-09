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

var IDXRunAnnCmd = &cobra.Command{
	Use:     "announcements",
	Aliases: []string{"ann", "a"},
	Short:   "Run idx announcements worker only",
	RunE:    idxWorkerAnn,
}

func idxWorkerAnn(cmd *cobra.Command, args []string) error {
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
		err = fmt.Errorf("load config: %v", err)
		return err
	}

	logger := log.Get(cfg.App)
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic", slog.Any("panic", r))
			return
		}
	}()

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

	idxWorker := idx.NewWorkerIdx(
		ctx,
		&cfg.App.IDX,
		deps.logger.With(slog.String("tag", "idx.Worker")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.annStore,
		deps.attStore,
		deps.client,
		deps.db,
		deps.stg,
	)
	defer idxWorker.Shutdown()

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
	logger.InfoContext(ctx, "received context done, stopping")

	return nil
}
