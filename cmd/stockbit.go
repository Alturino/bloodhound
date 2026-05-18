package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/db"
	"github.com/alturino/bloodhound/internal/log"
	"github.com/alturino/bloodhound/internal/telemetry"
)

var StocbitWorker = &cobra.Command{
	Use:     "sb run",
	Short:   "Run the stockbit worker service",
	RunE:    stockbitWorker,
	Aliases: []string{"sr"},
}

func stockbitWorker(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
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

	// stg, err := storage.NewStorage(&cfg.Storage, logger, telemetry.AppTelemetry.Tracer)
	// if err != nil {
	// 	err = fmt.Errorf("initialize storage: %w", err)
	// 	logger.ErrorContext(ctx, err.Error())
	// 	return err
	// }

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
	// stockbitStore := state.NewStockbitStore(database, logger)
	//
	// httpClient := httpclient.NewClient(cfg, telemetry.AppTelemetry.Tracer)

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

	return nil
}
