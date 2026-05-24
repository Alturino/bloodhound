package stockbit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	. "github.com/go-jet/jet/v2/postgres"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/attribute"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	. "github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/table"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/telemetry"
	"github.com/google/uuid"
)

// Store - Stockbit market detector state persistence
type Store interface {
	UpsertMarketDetector(
		ctx context.Context,
		summary models.MarketDetectorSummary,
		transactions []models.BrokerTransaction,
	) error
	StockCodes(ctx context.Context) ([]string, error)
	GetExistingTradeDates(ctx context.Context, symbol string) ([]time.Time, error)
	UpsertBrokerTransactions(ctx context.Context, transactions []models.BrokerTransaction) error
}

type stockbitStore struct {
	db     *sql.DB
	logger *slog.Logger
	tracer trace.Tracer
}

func NewStockbitStore(db *sql.DB, logger *slog.Logger, tracer trace.Tracer) Store {
	return &stockbitStore{db: db, logger: logger, tracer: tracer}
}

// UpsertMarketDetector inserts or updates market detector summaries and transactions
func (s *stockbitStore) UpsertMarketDetector(
	ctx context.Context,
	summary models.MarketDetectorSummary,
	transactions []models.BrokerTransaction,
) error {
	ctx, span := s.tracer.Start(ctx, "store.StockbitStore.UpsertMarketDetector")
	defer span.End()

	logger := s.logger.With(slog.String("tag", "store.StockbitStore.UpsertMarketDetector"))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		err = fmt.Errorf("StockbitStore.UpsertMarketDetector begin tx: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			err = fmt.Errorf("rollback: %w", err)
			if !errors.Is(err, sql.ErrTxDone) || !errors.Is(err, sql.ErrConnDone) {
				logger.ErrorContext(ctx, err.Error())
				telemetry.RecordError(span, err)
				return
			}
			logger.DebugContext(ctx, "transaction already committed", slog.Any("error", err))
			span.AddEvent("transaction already committed")
			return
		}
		logger.InfoContext(ctx, "transaction rolled back")
		span.AddEvent("transaction rolled back")
	}()

	var dbSummary model.MarketDetectorSummaries
	tDate, _ := time.Parse("2006-01-02", summary.TradeDate)

	// Convert decimal.Decimal to float64 for Jet if necessary,
	// or rely on decimal.Decimal implementing Valuer (which it does).
	totalValue, _ := summary.TotalValue.Float64()

	sumModel := model.MarketDetectorSummaries{
		Symbol:        summary.Symbol,
		TradeDate:     tDate,
		AccdistStatus: summary.AccDistStatus,
		TotalValue:    totalValue,
	}
	stmt := MarketDetectorSummaries.INSERT(
		MarketDetectorSummaries.AllColumns.Except(
			MarketDetectorSummaries.ID,
			MarketDetectorSummaries.CreatedAt,
		),
	).MODEL(sumModel).
		ON_CONFLICT(MarketDetectorSummaries.Symbol, MarketDetectorSummaries.TradeDate).
		DO_UPDATE(SET(
			MarketDetectorSummaries.AccdistStatus.SET(
				MarketDetectorSummaries.EXCLUDED.AccdistStatus,
			),
			MarketDetectorSummaries.TotalValue.SET(MarketDetectorSummaries.EXCLUDED.TotalValue),
		)).
		RETURNING(MarketDetectorSummaries.ID)
	if err := stmt.QueryContext(ctx, tx, &dbSummary); err != nil {
		err = fmt.Errorf("upsert summary: %w", err)
		telemetry.RecordError(span, err)
		return err
	}

	for _, t := range transactions {
		var dbTxns []model.BrokerTransactions
		side := t.Side
		invType := t.InvestorType
		avgPrice, _ := t.AvgPrice.Float64()
		freq := int32(t.Frequency)
		lots := t.Lots

		dbTxns = append(dbTxns, model.BrokerTransactions{
			Frequency:    freq,
			Lots:         lots,
			AvgPrice:     avgPrice,
			SummaryID:    dbSummary.ID,
			BrokerCode:   t.BrokerCode,
			InvestorType: invType,
			Side:         side,
			Symbol:       t.Symbol,
			TradeDate:    tDate,
		})

		insStmt := BrokerTransactions.INSERT(BrokerTransactions.AllColumns.Except(BrokerTransactions.ID)).
			MODELS(dbTxns).
			RETURNING(BrokerTransactions.ID).
			ON_CONFLICT(BrokerTransactions.Symbol, BrokerTransactions.TradeDate, BrokerTransactions.BrokerCode, BrokerTransactions.Side).
			DO_UPDATE(SET(
				BrokerTransactions.AvgPrice.SET(BrokerTransactions.EXCLUDED.AvgPrice),
				BrokerTransactions.Lots.SET(BrokerTransactions.EXCLUDED.Lots),
				BrokerTransactions.Frequency.SET(BrokerTransactions.EXCLUDED.Frequency),
			))
		if err := insStmt.QueryContext(ctx, tx, nil); err != nil {
			err = fmt.Errorf("upsert broker transactions: %w", err)
			telemetry.RecordError(span, err)
			return err
		}
	}

	logger.DebugContext(ctx, "commiting transaction")
	span.AddEvent("commiting transaction")
	if err := tx.Commit(); err != nil {
		err = fmt.Errorf("commit: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	logger.InfoContext(ctx, "commited transaction")
	span.AddEvent("commited transaction")

	return nil
}

func (s *stockbitStore) StockCodes(ctx context.Context) ([]string, error) {
	ctx, span := s.tracer.Start(ctx, "store.StockbitStore.GetStockCodesForMarketDetector")
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "store.StockbitStore.GetStockCodesForMarketDetector"),
	)

	logger.DebugContext(ctx, "fetching stock codes for market detector")
	span.AddEvent("fetching stock codes for market detector")

	var stockCodes []string
	since := time.Now().Add(-24 * time.Hour)
	stmt := fmt.Sprintf(
		"SELECT stock_code FROM announcements WHERE created_at > $1 GROUP BY stock_code ORDER BY stock_code ASC",
	)
	rows, err := s.db.QueryContext(ctx, stmt, since)
	if err != nil {
		err = fmt.Errorf("get stock codes: %w", err)
		telemetry.RecordError(span, err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			continue
		}
		stockCodes = append(stockCodes, code)
	}

	logger.DebugContext(ctx, "fetched stock codes", slog.Int("count", len(stockCodes)))
	span.AddEvent("fetched stock codes")

	return stockCodes, nil
}

// GetExistingTradeDates retrieves distinct trade dates for a given symbol
func (s *stockbitStore) GetExistingTradeDates(ctx context.Context, symbol string) ([]time.Time, error) {
	ctx, span := s.tracer.Start(ctx, "store.StockbitStore.GetExistingTradeDates")
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "store.StockbitStore.GetExistingTradeDates"),
	)

	logger.DebugContext(ctx, "getting existing trade dates for symbol", slog.String("symbol", symbol))
	span.AddEvent("getting existing trade dates for symbol", trace.WithAttributes(attribute.String("symbol", symbol)))

	var dates []time.Time

	stmt := fmt.Sprintf(
		"SELECT DISTINCT trade_date FROM broker_transactions WHERE symbol = $1 ORDER BY trade_date ASC",
	)
	rows, err := s.db.QueryContext(ctx, stmt, symbol)
	if err != nil {
		err = fmt.Errorf("get existing trade dates: %w", err)
		telemetry.RecordError(span, err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tradeDate time.Time
		if err := rows.Scan(&tradeDate); err != nil {
			continue
		}
		dates = append(dates, tradeDate)
	}

	logger.DebugContext(ctx, "fetched existing trade dates", slog.Int("count", len(dates)), slog.String("symbol", symbol))
	span.AddEvent("fetched existing trade dates", trace.WithAttributes(attribute.Int("count", len(dates)), attribute.String("symbol", symbol)))

	return dates, nil
}

// UpsertBrokerTransactions inserts or updates broker transactions
func (s *stockbitStore) UpsertBrokerTransactions(ctx context.Context, transactions []models.BrokerTransaction) error {
	ctx, span := s.tracer.Start(ctx, "store.StockbitStore.UpsertBrokerTransactions")
	defer span.End()

	logger := s.logger.With(slog.String("tag", "store.StockbitStore.UpsertBrokerTransactions"))

	if len(transactions) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		err = fmt.Errorf("StockbitStore.UpsertBrokerTransactions begin tx: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			err = fmt.Errorf("rollback: %w", err)
			if !errors.Is(err, sql.ErrTxDone) || !errors.Is(err, sql.ErrConnDone) {
				logger.ErrorContext(ctx, err.Error())
				telemetry.RecordError(span, err)
				return
			}
			logger.DebugContext(ctx, "transaction already committed", slog.Any("error", err))
			span.AddEvent("transaction already committed")
			return
		}
		logger.InfoContext(ctx, "transaction rolled back")
		span.AddEvent("transaction rolled back")
		return
	}()

	var dbTxns []model.BrokerTransactions
	for _, t := range transactions {
		side := t.Side
		invType := t.InvestorType
		avgPrice, _ := t.AvgPrice.Float64()
		freq := int32(t.Frequency)
		lots := t.Lots

		// Assuming we get the summary ID from the first transaction or we need a different approach
		// For now, we'll need to look up the summary ID based on symbol and trade date
		// This is a simplification - in reality, we'd need to join with market_detector_summaries
		tDate, _ := time.Parse("2006-01-02", t.TradeDate)

		// Get the summary ID for this symbol and trade date
		summaryStmt := fmt.Sprintf(
			"SELECT id FROM market_detector_summaries WHERE symbol = $1 AND trade_date = $2",
		)
		var summaryID uuid.UUID
		err := tx.QueryRowContext(ctx, summaryStmt, t.Symbol, tDate).Scan(&summaryID)
		if err != nil {
			if err == sql.ErrNoRows {
				// If no summary exists, we can't upsert transactions
				logger.WarnContext(ctx, "no market detector summary found for symbol and date", slog.String("symbol", t.Symbol), slog.String("trade_date", t.TradeDate))
				continue
			}
			err = fmt.Errorf("get summary ID: %w", err)
			telemetry.RecordError(span, err)
			return err
		}

		dbTxns = append(dbTxns, model.BrokerTransactions{
			Frequency:    freq,
			Lots:         lots,
			AvgPrice:     avgPrice,
			SummaryID:    summaryID,
			BrokerCode:   t.BrokerCode,
			InvestorType: invType,
			Side:         side,
			Symbol:       t.Symbol,
			TradeDate:    tDate,
		})
	}

	if len(dbTxns) == 0 {
		return nil
	}

	insStmt := BrokerTransactions.INSERT(BrokerTransactions.AllColumns.Except(BrokerTransactions.ID)).
		MODELS(dbTxns).
		RETURNING(BrokerTransactions.ID).
		ON_CONFLICT(BrokerTransactions.Symbol, BrokerTransactions.TradeDate, BrokerTransactions.BrokerCode, BrokerTransactions.Side).
		DO_UPDATE(SET(
			BrokerTransactions.AvgPrice.SET(BrokerTransactions.EXCLUDED.AvgPrice),
			BrokerTransactions.Lots.SET(BrokerTransactions.EXCLUDED.Lots),
			BrokerTransactions.Frequency.SET(BrokerTransactions.EXCLUDED.Frequency),
		))

	if err := insStmt.QueryContext(ctx, tx, nil); err != nil {
		err = fmt.Errorf("upsert broker transactions: %w", err)
		telemetry.RecordError(span, err)
		return err
	}

	logger.DebugContext(ctx, "commiting transaction")
	span.AddEvent("commiting transaction")
	if err := tx.Commit(); err != nil {
		err = fmt.Errorf("commit: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	logger.InfoContext(ctx, "commited transaction")
	span.AddEvent("commited transaction")

	return nil
}
