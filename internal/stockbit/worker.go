package stockbit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/store"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type Stockbit struct {
	config        *config.Config
	logger        *slog.Logger
	tracer        trace.Tracer
	metrics       *telemetry.Metrics
	client        Client
	stockbitStore store.StockbitStore
}

func (w Stockbit) Start(ctx context.Context) error {
	interval := w.config.App.IDX.Scheduler.Interval

	logger := w.logger.With(
		slog.String("tag", "stockbit.WorkerStockbit.Start"),
		slog.Duration("interval", interval),
	)

	logger.InfoContext(ctx, "started background Stockbit worker")

	if err := w.Process(ctx); err != nil {
		err = fmt.Errorf("initial processing: %w", err)
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "received context done, stopping", slog.Any("error", ctx.Err()))
			return ctx.Err()
		case <-ticker.C:
			if err := w.Process(ctx); err != nil {
				return err
			}
		}
	}
}

func (w Stockbit) Process(ctx context.Context) error {
	ctx, span := w.tracer.Start(
		ctx,
		"stockbit.WorkerStockbit.Process",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := w.logger.With(slog.String("tag", "stockbit.WorkerStockbit.Process"))

	span.AddEvent("processing market detector")

	stockCodes, err := w.stockbitStore.StockCodes(ctx)
	if err != nil {
		err = fmt.Errorf("get stock codes: %v", err)
		return err
	}

	w.metrics.SbStockCodesTotal.Record(ctx, int64(len(stockCodes)))

	if len(stockCodes) == 0 {
		logger.InfoContext(ctx, "no stock codes found for market detector sync")
		span.AddEvent("no stock codes found")
		return nil
	}

	logger.InfoContext(ctx, "starting market detector sync", slog.Int(constants.Count, len(stockCodes)))
	span.AddEvent("starting market detector sync")

	for _, symbol := range stockCodes {
		if err := w.syncMarketDetector(ctx, symbol); err != nil {
			logger.ErrorContext(ctx, "sync market detector",
				slog.String(constants.Symbol, symbol),
				slog.Any("error", err))
			continue
		}
	}

	span.AddEvent("processed market detector")
	return nil
}

func (w Stockbit) syncMarketDetector(ctx context.Context, symbol string) error {
	start := time.Now()
	defer func() {
		w.metrics.SbSyncDuration.Record(ctx, float64(time.Since(start).Milliseconds()))
	}()

	ctx, span := w.tracer.Start(ctx, "stockbit.WorkerStockbit.syncMarketDetector")
	defer span.End()

	span.AddEvent("syncing market detector")

	dateStr := time.Now().Format("2006-01-02")

	resp, err := w.client.FetchMarketDetector(ctx, symbol, dateStr, dateStr)
	if err != nil {
		return err
	}

	summary := models.MarketDetectorSummary{
		Symbol:        symbol,
		TradeDate:     dateStr,
		AccDistStatus: resp.Data.BandarDetector.BrokerAccDist,
		TotalValue:    resp.Data.BandarDetector.Value,
	}

	var txns []models.BrokerTransaction
	for _, b := range resp.Data.BrokerSummary.BrokersBuy {
		lots := b.BLot.IntPart()
		txns = append(txns, models.BrokerTransaction{
			Symbol:       symbol,
			TradeDate:    dateStr,
			BrokerCode:   b.BrokerCode,
			Side:         "BUY",
			Lots:         lots,
			Frequency:    b.Freq,
			InvestorType: b.InvestorType,
			AvgPrice:     b.BuyAvgPrice,
		})
	}

	for _, b := range resp.Data.BrokerSummary.BrokersSell {
		lots := b.SLot.IntPart()
		txns = append(txns, models.BrokerTransaction{
			Symbol:       symbol,
			TradeDate:    dateStr,
			BrokerCode:   b.BrokerCode,
			Side:         "SELL",
			Lots:         lots,
			Frequency:    b.Freq,
			InvestorType: b.InvestorType,
			AvgPrice:     b.SellAvgPrice,
		})
	}

	if err := w.stockbitStore.UpsertMarketDetector(ctx, summary, txns); err != nil {
		err = fmt.Errorf("upsert: %v", err)
		return err
	}

	w.metrics.SbSymbolsSynced.Add(ctx, 1, metric.WithAttributes(
		attribute.String(constants.Status, "success"),
	))

	span.AddEvent("synced market detector")
	return nil
}
