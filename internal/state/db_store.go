package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	. "github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/db/.gen/postgres/public/model"
	. "github.com/alturino/bloodhound/internal/db/.gen/postgres/public/table"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// DBStore implements the Store interface using PostgreSQL and go-jet
type DBStore struct {
	db     *sql.DB
	logger *slog.Logger
	tracer trace.Tracer
}

func (s *DBStore) ShouldUpdate(ctx context.Context) (bool, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"state.DBStore.ShouldUpdate",
		trace.WithAttributes(attribute.String("tag", "state.DBStore.ShouldUpdate")),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	now := time.Now()
	logger := s.logger.With(
		slog.String("tag", "state.DBStore.ShouldUpdate"),
		slog.Time("current_time", now),
	)

	logger.DebugContext(ctx, "get latest announcement")
	span.AddEvent("get latest announcement")
	var ann models.Announcement
	if err := Announcements.SELECT(Announcements.AllColumns).
		ORDER_BY(Announcements.AnnouncementDate.DESC()).
		LIMIT(1).
		QueryContext(ctx, s.db, &ann); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return true, nil
		}
		err = fmt.Errorf("state.DBStore.ShouldUpdate get latest announcement: %w", err)
		return false, err
	}
	logger.InfoContext(ctx, "got latest announcements")
	span.AddEvent("got latest announcements")

	logger.DebugContext(ctx, "is latest announcement outdated")
	span.AddEvent("is latest announcement outdated")
	isOutdated := ann.AnnouncementDate.Before(now)
	logger = logger.With(
		slog.Time("latest_announcement_date", ann.AnnouncementDate),
		slog.Duration("now_latest_diff", now.Sub(ann.AnnouncementDate)),
		slog.Bool("isOutdated", isOutdated),
	)
	logger.InfoContext(ctx, "latest announcement")
	span.AddEvent("latest announcement")

	return isOutdated, nil
}

// NewDBStore creates a new PostgreSQL-backed store
func NewDBStore(db *sql.DB, logger *slog.Logger) *DBStore {
	if logger == nil {
		logger = slog.Default().With(slog.String("tag", "state.DBStore"))
	}
	return &DBStore{db: db, logger: logger, tracer: telemetry.AppTelemetry.Tracer}
}

// IsProcessed checks if an announcement ID exists in the database by its IDX ID
func (s *DBStore) IsProcessed(ctx context.Context, idxID string) (bool, error) {
	ctx, span := s.tracer.Start(ctx, "state.DBStore.IsProcessed")
	defer span.End()

	var announcement model.Announcements
	isExistStmt := SELECT(Announcements.AllColumns).
		FROM(Announcements).
		WHERE(Announcements.IdxID.EQ(String(idxID))).
		LIMIT(1)
	if err := isExistStmt.QueryContext(ctx, s.db, &announcement); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return false, nil
		}
		err = fmt.Errorf("DBStore.IsProcessed: %w", err)
		telemetry.RecordError(span, err)
		return false, err
	}
	return true, nil
}

// RecordAnnouncement saves announcement metadata
func (s *DBStore) RecordAnnouncement(ctx context.Context, ann models.Announcement) error {
	ctx, span := s.tracer.Start(ctx, "state.DBStore.RecordAnnouncement")
	defer span.End()

	announcement := ann.ToAnnouncements()
	if err := Announcements.INSERT(Announcements.AllColumns.Except(Announcements.ID, Announcements.CreatedAt)).
		MODEL(announcement).
		RETURNING(Announcements.AllColumns).
		QueryContext(ctx, s.db, &announcement); err != nil {
		err = fmt.Errorf("DBStore.RecordAnnouncement insert announcement: %w", err)
		telemetry.RecordError(span, err)
		return err
	}

	return nil
}

// RecordAttachment saves attachment metadata
func (s *DBStore) RecordAttachment(
	ctx context.Context,
	idxID string,
	att models.Attachment,
	checksum string,
	storagePath string,
) error {
	ctx, span := s.tracer.Start(ctx, "state.DBStore.RecordAttachment")
	defer span.End()

	attachment := att.ToAttachments(idxID, checksum, storagePath)
	if err := Attachments.INSERT(Attachments.AllColumns.Except(Attachments.ID)).
		MODEL(attachment).
		RETURNING(Attachments.AllColumns).
		QueryContext(ctx, s.db, &attachment); err != nil {
		err = fmt.Errorf("DBStore.RecordAttachment: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	return nil
}

// UpsertMarketDetector inserts or updates market detector summaries and transactions
func (s *DBStore) UpsertMarketDetector(
	ctx context.Context,
	summary models.MarketDetectorSummary,
	transactions []models.BrokerTransaction,
) error {
	ctx, span := s.tracer.Start(ctx, "state.DBStore.UpsertMarketDetector")
	defer span.End()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		err = fmt.Errorf("DBStore.UpsertMarketDetector begin tx: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	defer tx.Rollback()

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

	return tx.Commit()
}
