package stockbit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
)

type StockbitHistoricalScheduler struct {
	config *config.Config
	logger *slog.Logger
	tracer trace.Tracer
	client Client
	store  Store
	cron   *cron.Cron
}

func NewStockbitHistoricalScheduler(
	cfg *config.Config,
	logger *slog.Logger,
	tracer trace.Tracer,
	client Client,
	store Store,
) *StockbitHistoricalScheduler {
	return &StockbitHistoricalScheduler{
		config: cfg,
		logger: logger,
		tracer: tracer,
		client: client,
		store:  store,
		cron:   cron.New(),
	}
}

func (s *StockbitHistoricalScheduler) Start(ctx context.Context) error {
	logger := s.logger.With(slog.String("tag", "scheduler.StockbitHistoricalScheduler.Start"))

	cronExpr := s.config.Scheduler.HistoricalCron
	if cronExpr == "" {
		cronExpr = "0 18 * * *"
	}

	_, err := s.cron.AddFunc(cronExpr, func() {
		if err := s.Process(ctx); err != nil {
			logger.ErrorContext(ctx, "historical sync error", slog.Any("error", err))
		}
	})
	if err != nil {
		return fmt.Errorf("add cron job: %w", err)
	}

	s.cron.Start()
	logger.InfoContext(ctx, "started historical scheduler", slog.String("cron", cronExpr))

	<-ctx.Done()
	s.cron.Stop()
	logger.InfoContext(ctx, "stopped historical scheduler")
	return ctx.Err()
}

func (s *StockbitHistoricalScheduler) Process(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "scheduler.StockbitHistoricalScheduler.Process")
	defer span.End()

	// logger := s.logger.With(slog.String("tag", "scheduler.StockbitHistoricalScheduler.Process"))

	// symbols, err := s.store.GetStockCodesForMarketDetector(ctx)
	// if err != nil {
	// 	return fmt.Errorf("get stock codes: %w", err)
	// }

	// if len(symbols) == 0 {
	// 	logger.InfoContext(ctx, "no symbols found for historical sync")
	// 	return nil
	// }

	// logger.InfoContext(ctx, "starting historical sync", slog.Int("symbol_count", len(symbols)))
	//
	// for _, symbol := range symbols {
	// 	if err := s.syncSymbol(ctx, symbol); err != nil {
	// 		logger.ErrorContext(ctx, "sync symbol error",
	// 			slog.String("symbol", symbol),
	// 			slog.Any("error", err))
	// 		continue
	// 	}
	// }
	//
	return nil
}

func (s *StockbitHistoricalScheduler) syncSymbol(ctx context.Context, symbol string) error {
	ctx, span := s.tracer.Start(ctx, "scheduler.StockbitHistoricalScheduler.syncSymbol")
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "scheduler.StockbitHistoricalScheduler.syncSymbol"),
		slog.String("symbol", symbol),
	)

	existingDates, err := s.store.GetExistingTradeDates(ctx, symbol)
	if err != nil {
		return fmt.Errorf("get existing dates: %w", err)
	}

	today := time.Now()
	oneYearAgo := today.AddDate(-1, 0, 0)

	var dateFrom, dateTo string

	if len(existingDates) == 0 {
		dateFrom = ""
		dateTo = ""
		logger.InfoContext(ctx, "first run - using period RT_PERIOD_LAST_1_YEAR")
	} else {
		existingDateSet := make(map[string]bool)
		for _, d := range existingDates {
			existingDateSet[d.Format("2006-01-02")] = true
		}

		for d := oneYearAgo; !d.After(today); d = d.AddDate(0, 0, 1) {
			dateStr := d.Format("2006-01-02")
			if !existingDateSet[dateStr] {
				dateFrom = dateStr
				dateTo = today.Format("2006-01-02")
				break
			}
		}

		if dateFrom == "" {
			logger.InfoContext(ctx, "no missing dates to sync")
			return nil
		}
		logger.InfoContext(ctx, "incremental sync",
			slog.String("from", dateFrom),
			slog.String("to", dateTo))
	}

	// resp, err := s.client.FetchBrokerActivity(ctx, symbol, dateFrom, dateTo)
	// if err != nil {
	// 	return fmt.Errorf("fetch broker activity: %w", err)
	// }

	// var txns []models.BrokerTransaction
	// for _, record := range resp.Data.Records {
	// 	if record.TradeActivity.BuySummary.Freq > 0 {
	// 		tradeDate, _ := time.Parse("2006-01-02", record.Date)
	// 		txns = append(txns, models.BrokerTransaction{
	// 			Symbol:       symbol,
	// 			TradeDate:    tradeDate.Format("2006-01-02"),
	// 			BrokerCode:   record.BrokerCode,
	// 			Side:         "BUY",
	// 			InvestorType: "ALL",
	// 			Frequency:    record.TradeActivity.BuySummary.Freq,
	// 			Lots:         record.TradeActivity.BuySummary.Lot,
	// 			AvgPrice:     decimal.NewFromFloat(record.TradeActivity.BuySummary.AvgPrice),
	// 		})
	// 	}
	// 	if record.TradeActivity.SellSummary.Freq > 0 {
	// 		tradeDate, _ := time.Parse("2006-01-02", record.Date)
	// 		txns = append(txns, models.BrokerTransaction{
	// 			Symbol:       symbol,
	// 			TradeDate:    tradeDate.Format("2006-01-02"),
	// 			BrokerCode:   record.BrokerCode,
	// 			Side:         "SELL",
	// 			InvestorType: "ALL",
	// 			Frequency:    record.TradeActivity.SellSummary.Freq,
	// 			Lots:         record.TradeActivity.SellSummary.Lot,
	// 			AvgPrice:     decimal.NewFromFloat(record.TradeActivity.SellSummary.AvgPrice),
	// 		})
	// 	}
	// }

	// if len(txns) > 0 {
	// 	if err := s.store.UpsertBrokerTransactions(ctx, txns); err != nil {
	// 		return fmt.Errorf("upsert transactions: %w", err)
	// 	}
	// 	logger.InfoContext(ctx, "synced transactions", slog.Int("count", len(txns)))
	// }

	return nil
}
