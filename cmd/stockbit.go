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
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/db"
	"github.com/alturino/bloodhound/internal/httpclient"
	"github.com/alturino/bloodhound/internal/log"
	"github.com/alturino/bloodhound/internal/stockbit"
	"github.com/alturino/bloodhound/internal/telemetry"
)

var StockbitWorker = &cobra.Command{
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
		err = fmt.Errorf("load config: %v", err)
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
		err = fmt.Errorf("initialize telemetry: %v", err)
		logger.ErrorContext(ctx, err.Error())
		return err
	}
	defer func() {
		if err := tmt.Shutdown(ctx); err != nil {
			err = fmt.Errorf("shutdown telemetry: %v", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
	}()

	// stg, err := blobstorage.NewStorage(&cfg.Storage, logger, telemetry.AppTelemetry.Tracer)
	// if err != nil {
	// 	err = fmt.Errorf("initialize storage: %v", err)
	// 	logger.ErrorContext(ctx, err.Error())
	// 	return err
	// }

	database, err := db.Get(ctx, &cfg.Database)
	if err != nil {
		err = fmt.Errorf("initialize database %v", err)
		logger.ErrorContext(ctx, err.Error())
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			err = fmt.Errorf("close database: %v", err)
			logger.ErrorContext(ctx, err.Error())
			return
		}
	}()

	httpClient := httpclient.NewClient(cfg, telemetry.AppTelemetry.Tracer)
	stockbitClient := stockbit.NewClient(
		httpClient,
		&cfg.App.Stockbit,
		logger.With(slog.String("tag", "stockbithistorical.Client")),
		telemetry.AppTelemetry.Tracer,
	)
	stockbitStore := stockbit.NewStockbitStore(database, logger, telemetry.AppTelemetry.Tracer)
	historicalScheduler := stockbit.NewStockbitHistoricalScheduler(
		cfg,
		logger.With(slog.String("tag", "scheduler.StockbitHistoricalScheduler")),
		telemetry.AppTelemetry.Tracer,
		stockbitClient,
		stockbitStore,
	)

	stWorker := stockbit.NewWorker(
		cfg,
		logger,
		telemetry.AppTelemetry.Tracer,
		stockbitStore,
		stockbitClient,
	)
	if err := stWorker.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.ErrorContext(ctx, "stockbit worker error", slog.Any("error", err))
		return err
	}
	go func() {
		if err := historicalScheduler.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(ctx, "historical scheduler error", slog.Any("error", err))
		}
	}()
	if err := stWorker.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.ErrorContext(ctx, "stockbit worker error", slog.Any("error", err))
		return err
	}

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

	return nil
}
