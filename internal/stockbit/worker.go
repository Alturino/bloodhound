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
	"github.com/alturino/bloodhound/internal/telemetry"
)

type Worker struct {
	config  *config.Config
	logger  *slog.Logger
	ctx     context.Context
	tracer  trace.Tracer
	metrics *telemetry.Metrics
	client  Client
	store   Store
}

func NewWorker(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
	tracer trace.Tracer,
	store Store,
	client Client,
) *Worker {
	w := &Worker{
		config: cfg,
		ctx:    ctx,
		logger: logger,
		tracer: tracer,
		client: client,
		store:  store,
	}
	go func() {
		w.Start()
	}()
	return w
}

func (w *Worker) Start() {
	interval := w.config.Scheduler.Interval
	logger := w.logger.With(
		slog.String("tag", "stockbit.WorkerStockbit.Start"),
		slog.Duration("interval", interval),
	)

	logger.InfoContext(w.ctx, "started background Stockbit worker")
	w.looper(w.Process)
}

func (w *Worker) looper(run func(ctx context.Context) error) {
	interval := w.config.Scheduler.Interval
	tick := time.Tick(interval)
	for {
		select {
		case <-w.ctx.Done():
			w.logger.InfoContext(w.ctx, "stopping", slog.Any("error", w.ctx.Err()))
			return
		case <-tick:
			if err := run(w.ctx); err != nil {
				err = fmt.Errorf("run: %v", err)
				w.logger.ErrorContext(w.ctx, err.Error(), slog.Any("error", err))
				return
			}
		}
	}
}

func (w *Worker) Process(ctx context.Context) error {
	ctx, span := w.tracer.Start(
		ctx,
		"stockbit.WorkerStockbit.Process",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := w.logger.With(slog.String("tag", "stockbit.WorkerStockbit.Process"))

	stockCodes, err := w.store.StockCodes(ctx)
	if err != nil {
		err = fmt.Errorf("get stock codes: %v", err)
		return err
	}

	w.metrics.SbStockCodesTotal.Record(ctx, int64(len(stockCodes)))

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

func (w *Worker) syncMarketDetector(ctx context.Context, symbol string) error {
	ctx, span := w.tracer.Start(ctx, "stockbit.WorkerStockbit.syncMarketDetector")
	defer span.End()

	// dateStr := time.Now().Format("2006-01-02")

	// logger := w.logger.With(
	// 	slog.String("tag", "stockbit.WorkerStockbit.syncMarketDetector"),
	// 	slog.String("symbol", symbol),
	// )

	// resp, err := w.client.FetchMarketDetector(ctx, symbol, dateStr, dateStr)
	// if err != nil {
	// 	return err
	// }

	// summary := MarketDetectorSummary{
	// 	Symbol:        symbol,
	// 	TradeDate:     dateStr,
	// 	AccDistStatus: resp.Data.BandarDetector.BrokerAccDist,
	// 	TotalValue:    resp.Data.BandarDetector.Value,
	// }

	// var txns []BrokerTransaction
	// for _, b := range resp.Data.BrokerSummary.BrokersBuy {
	// 	lots := b.BLot.IntPart()
	// 	txns = append(txns, BrokerTransaction{
	// 		Symbol:       symbol,
	// 		TradeDate:    dateStr,
	// 		BrokerCode:   b.BrokerCode,
	// 		Side:         "BUY",
	// 		Lots:         lots,
	// 		Frequency:    b.Freq,
	// 		InvestorType: b.InvestorType,
	// 		AvgPrice:     b.BuyAvgPrice,
	// 	})
	// }

	// for _, b := range resp.Data.BrokerSummary.BrokersSell {
	// 	lots := b.SLot.IntPart()
	// 	txns = append(txns, models.BrokerTransaction{
	// 		Symbol:       symbol,
	// 		TradeDate:    dateStr,
	// 		BrokerCode:   b.BrokerCode,
	// 		Side:         "SELL",
	// 		Lots:         lots,
	// 		Frequency:    b.Freq,
	// 		InvestorType: b.InvestorType,
	// 		AvgPrice:     b.SellAvgPrice,
	// 	})
	// }

	// if err := w.store.UpsertMarketDetector(ctx, summary, txns); err != nil {
	// 	err = fmt.Errorf("upsert: %v", err)
	// 	return err
	// }

	w.metrics.SbSymbolsSynced.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", "success"),
	))

	return nil
}
