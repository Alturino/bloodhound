package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	. "github.com/go-jet/jet/v2/postgres"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	. "github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/table"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// StockbitStore - Stockbit market detector state persistence
type StockbitStore interface {
	UpsertMarketDetector(
		ctx context.Context,
		summary models.MarketDetectorSummary,
		transactions []models.BrokerTransaction,
	) error
	StockCodes(ctx context.Context) ([]string, error)
}

type stockbitStore struct {
	db     *sql.DB
	logger *slog.Logger
	tracer trace.Tracer
}

func NewStockbitStore(db *sql.DB, logger *slog.Logger, tracer trace.Tracer) StockbitStore {
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

	span.AddEvent("upserting market detector")

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		err = fmt.Errorf("StockbitStore.UpsertMarketDetector begin tx: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			err = fmt.Errorf("rollback: %v", err)
			if !errors.Is(err, sql.ErrTxDone) || !errors.Is(err, sql.ErrConnDone) {
				telemetry.RecordError(span, err)
				return
			}
			logger.DebugContext(ctx, "transaction already committed", slog.Any("error", err))
			return
		}
		logger.WarnContext(ctx, "transaction rolled back")
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

	logger.DebugContext(ctx, "committing transaction")
	span.AddEvent("committing transaction")
	if err := tx.Commit(); err != nil {
		err = fmt.Errorf("commit: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	logger.DebugContext(ctx, "committed transaction")
	span.AddEvent("committed transaction")

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

	stmt := SELECT(Announcements.StockCode).
		FROM(Announcements).
		WHERE(Announcements.CreatedAt.GT(TimestampzT(since))).
		GROUP_BY(Announcements.StockCode).
		ORDER_BY(Announcements.StockCode.ASC())

	var results []struct {
		StockCode string
	}
	err := stmt.QueryContext(ctx, s.db, &results)
	if err != nil {
		err = fmt.Errorf("get stock codes: %w", err)
		telemetry.RecordError(span, err)
		return nil, err
	}

	for _, r := range results {
		stockCodes = append(stockCodes, r.StockCode)
	}

	logger.DebugContext(ctx, "fetched stock codes", slog.Int(constants.Count, len(stockCodes)))
	span.AddEvent("fetched stock codes")

	return stockCodes, nil
}
