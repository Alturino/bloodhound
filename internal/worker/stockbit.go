package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/pkg/stockbit"
)

type WorkerStockbit struct {
	config         *config.Config
	logger        *slog.Logger
	tracer        trace.Tracer
	stockbitClient stockbit.Client
	stateStore    state.Store
}

func (w WorkerStockbit) Start(ctx context.Context) error {
	interval := w.config.Scheduler.Interval

	logger := w.logger.With(
		slog.String("tag", "worker.WorkerStockbit.Start"),
		slog.Duration("interval", interval),
	)

	logger.InfoContext(ctx, "started background Stockbit worker")

	if err := w.Process(ctx); err != nil {
		err = fmt.Errorf("initial processing: %w", err)
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return err
	}

	ticker := time.Tick(interval)
	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "stopping background Stockbit worker", slog.Any("error", ctx.Err()))
			return ctx.Err()
		case <-ticker:
			if err := w.Process(ctx); err != nil {
				return err
			}
		}
	}
}

func (w WorkerStockbit) Process(ctx context.Context) error {
	ctx, span := w.tracer.Start(
		ctx,
		"worker.WorkerStockbit.Process",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.WorkerStockbit.Process"))

	stockCodes, err := w.stateStore.GetStockCodesForMarketDetector(ctx)
	if err != nil {
		err = fmt.Errorf("get stock codes: %w", err)
		return err
	}

	if len(stockCodes) == 0 {
		logger.InfoContext(ctx, "no stock codes found for market detector sync")
		return nil
	}

	logger.InfoContext(ctx, "starting market detector sync", slog.Int("count", len(stockCodes)))

	for _, symbol := range stockCodes {
		if err := w.syncMarketDetector(ctx, symbol); err != nil {
			logger.ErrorContext(ctx, "sync market detector",
				slog.String("symbol", symbol),
				slog.Any("error", err))
			continue
		}
	}

	return nil
}

func (w WorkerStockbit) syncMarketDetector(ctx context.Context, symbol string) error {
	ctx, span := w.tracer.Start(ctx, "worker.WorkerStockbit.syncMarketDetector")
	defer span.End()

	dateStr := time.Now().Format("2006-01-02")

	_ = w.logger.With(slog.String("tag", "worker.WorkerStockbit.syncMarketDetector"))

	resp, err := w.stockbitClient.FetchMarketDetector(ctx, symbol, dateStr, dateStr)
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
			Side:        "BUY",
			Lots:        lots,
			Frequency:   b.Freq,
			InvestorType: b.InvestorType,
			AvgPrice:    b.BuyAvgPrice,
		})
	}

	for _, b := range resp.Data.BrokerSummary.BrokersSell {
		lots := b.SLot.IntPart()
		txns = append(txns, models.BrokerTransaction{
			Symbol:       symbol,
			TradeDate:    dateStr,
			BrokerCode:   b.BrokerCode,
			Side:        "SELL",
			Lots:        lots,
			Frequency:   b.Freq,
			InvestorType: b.InvestorType,
			AvgPrice:    b.SellAvgPrice,
		})
	}

	if err := w.stateStore.UpsertMarketDetector(ctx, summary, txns); err != nil {
		err = fmt.Errorf("upsert: %w", err)
		return err
	}

	return nil
}