package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/db"
	"github.com/alturino/bloodhound/internal/httpclient"
	"github.com/alturino/bloodhound/internal/log"
	"github.com/alturino/bloodhound/internal/stockbit"
	"github.com/alturino/bloodhound/internal/telemetry"
)

var HistoricalCmd = &cobra.Command{
	Use:   "broker-historical",
	Short: "Fetch broker activity historical data",
	Long:  `Fetch broker activity historical data from Stockbit API and store in database`,
	RunE:  runHistorical,
}

func runHistorical(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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

	stockbitStore := stockbit.NewStockbitStore(database, logger, telemetry.AppTelemetry.Tracer)

	httpClient := httpclient.NewClient(cfg, telemetry.AppTelemetry.Tracer)
	stockbitHistoricalClient := stockbit.NewClient(
		httpClient,
		&cfg.App.Stockbit,
		logger.With(slog.String("tag", "stockbithistorical.Client")),
		telemetry.AppTelemetry.Tracer,
	)

	historicalScheduler := stockbit.NewStockbitHistoricalScheduler(
		cfg,
		logger.With(slog.String("tag", "scheduler.StockbitHistoricalScheduler")),
		telemetry.AppTelemetry.Tracer,
		stockbitHistoricalClient,
		stockbitStore,
	)

	logger.InfoContext(ctx, "starting broker activity historical sync")

	if err := historicalScheduler.Process(ctx); err != nil {
		err = fmt.Errorf("historical sync: %w", err)
		logger.ErrorContext(ctx, err.Error())
		return err
	}

	logger.InfoContext(ctx, "broker activity historical sync completed")
	return nil
}
