package stockbit

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Store - Stockbit market detector state persistence
type Store interface {
	UpsertMarketDetector(
		ctx context.Context,
		summary MarketDetectorSummary,
		transactions []MarketDetectorBrokerTransaction,
	) error
	StockCodes(ctx context.Context) ([]string, error)
	GetExistingTradeDates(ctx context.Context, symbol string) ([]time.Time, error)
	UpsertBrokerTransactions(ctx context.Context, transactions []HistoricalBrokerTransaction) error
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
	summary MarketDetectorSummary,
	transactions []MarketDetectorBrokerTransaction,
) error {
	return errors.ErrUnsupported
}

func (s *stockbitStore) StockCodes(ctx context.Context) ([]string, error) {
	return nil, errors.ErrUnsupported
}

// GetExistingTradeDates retrieves distinct trade dates for a given symbol
func (s *stockbitStore) GetExistingTradeDates(
	ctx context.Context,
	symbol string,
) ([]time.Time, error) {
	return nil, errors.ErrUnsupported
}

// UpsertBrokerTransactions inserts or updates broker transactions
func (s *stockbitStore) UpsertBrokerTransactions(
	ctx context.Context,
	transactions []HistoricalBrokerTransaction,
) error {
	return errors.ErrUnsupported
}
