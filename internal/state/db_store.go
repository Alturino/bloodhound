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

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	. "github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/table"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// DBStore implements the Store interface using PostgreSQL and go-jet
type DBStore struct {
	db     *sql.DB
	logger *slog.Logger
	tracer trace.Tracer
}

// NewDBStore creates a new PostgreSQL-backed store
func NewDBStore(db *sql.DB, logger *slog.Logger) *DBStore {
	if logger == nil {
		logger = slog.Default().With(slog.String("tag", "state.DBStore"))
	}
	return &DBStore{db: db, logger: logger, tracer: telemetry.AppTelemetry.Tracer}
}

func (s DBStore) IsExists(ctx context.Context) (bool, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"state.DBStore.isExists",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String("tag", "state.DBStore.IsExists")),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "state.DBStore.IsExists"))

	logger.DebugContext(ctx, "checking announcements")
	span.AddEvent("checking announcements")
	var isExists struct{ bool }
	isEmptyStmt := SELECT(EXISTS(Announcements.SELECT(Announcements.AllColumns).LIMIT(1)))
	if err := isEmptyStmt.QueryContext(ctx, s.db, &isExists); err != nil {
		err = fmt.Errorf("checking announcements: %w", err)
		telemetry.RecordError(span, err)
		return false, err
	}
	logger = logger.With(slog.Bool("is_exists", isExists.bool))
	logger.DebugContext(ctx, "checked announcements")
	span.AddEvent("checked announcements")

	return isExists.bool, nil
}

func (s DBStore) LatestAnnouncement(ctx context.Context) (model.Announcements, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"state.DBStore.LatestAnnouncement",
		trace.WithAttributes(attribute.String("tag", "state.DBStore.LatestAnnouncement")),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	now := time.Now()
	logger := s.logger.With(
		slog.String("tag", "state.DBStore.LatestAnnouncement"),
		slog.Time("current_time", now),
	)

	logger.DebugContext(ctx, "get latest announcement")
	span.AddEvent("get latest announcement")
	var ann model.Announcements
	if err := Announcements.SELECT(Announcements.AllColumns).
		ORDER_BY(Announcements.Date.DESC()).
		LIMIT(1).
		QueryContext(ctx, s.db, &ann); err != nil {
		err = fmt.Errorf("get latest announcement: %w", err)
		telemetry.RecordError(span, err)
		return model.Announcements{}, err
	}
	logger.InfoContext(ctx, "got latest announcements")
	span.AddEvent("got latest announcements")

	return ann, nil
}

// IsProcessed checks if an announcement ID exists in the database by its IDX ID
func (s DBStore) IsProcessed(ctx context.Context, idxID string) (bool, error) {
	ctx, span := s.tracer.Start(ctx, "state.DBStore.IsProcessed")
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "state.DBStore.IsProcessed"),
		slog.String("idx_announcement_id", idxID),
	)

	logger.DebugContext(ctx, "is announcement processed")
	span.AddEvent("is announcement processed")
	var announcement model.Announcements
	isExistStmt := SELECT(Announcements.AllColumns).
		FROM(Announcements).
		WHERE(Announcements.IdxID.EQ(String(idxID))).
		LIMIT(1)
	if err := isExistStmt.QueryContext(ctx, s.db, &announcement); err != nil {
		err = fmt.Errorf("is announcement processed: %w", err)
		if errors.Is(err, qrm.ErrNoRows) {
			return false, nil
		}
		telemetry.RecordError(span, err)
		return false, err
	}
	logger = logger.With(slog.Bool("is_exist", true))
	logger.InfoContext(ctx, "checked announcement")
	span.AddEvent("checked announcement")

	return true, nil
}

// RecordAnnouncement saves announcement metadata
func (s DBStore) RecordAnnouncement(ctx context.Context, ann models.Announcement) error {
	ctx, span := s.tracer.Start(
		ctx,
		"state.DBStore.RecordAnnouncement",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "state.DBStore.RecordAnnouncement"))
	if logger.Enabled(ctx, slog.LevelDebug) {
		logger = logger.With(slog.Any("announceent", ann))
	}

	span.AddEvent("recording announcement")
	announcement := ann.ToAnnouncements()
	insertionColumn := Announcements.AllColumns.Except(Announcements.DefaultColumns)
	if err := Announcements.INSERT(insertionColumn).
		ON_CONFLICT(Announcements.IdxID).
		DO_NOTHING().
		MODEL(announcement).
		RETURNING(Announcements.AllColumns).
		QueryContext(ctx, s.db, &announcement); err != nil {
		err = fmt.Errorf("recording announcement: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	logger.InfoContext(ctx, "recorded announcement")
	span.AddEvent("recorded announcement")

	return nil
}

// RecordAttachment saves attachment metadata
func (s DBStore) RecordAttachment(
	ctx context.Context,
	idxID string,
	att models.Attachment,
	checksum string,
	storagePath string,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"state.DBStore.RecordAttachment",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "state.DBStore.RecordAttachment"),
		slog.String("idx_announcement_id", idxID),
		slog.String("attachment_name", att.OriginalFilename),
		slog.String("attachment_storage_path", storagePath),
	)

	logger.InfoContext(ctx, "recording attachment")
	span.AddEvent("recording attachment")
	attachment := att.ToAttachments(idxID, checksum, storagePath)
	if err := Attachments.INSERT(Attachments.AllColumns.Except(Attachments.ID)).
		MODEL(attachment).
		RETURNING(Attachments.AllColumns).
		QueryContext(ctx, s.db, &attachment); err != nil {
		err = fmt.Errorf("recording attachment: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	logger.InfoContext(ctx, "recorded attachment")
	span.AddEvent("recorded attachment")

	return nil
}

// UpsertMarketDetector inserts or updates market detector summaries and transactions
func (s DBStore) UpsertMarketDetector(
	ctx context.Context,
	summary models.MarketDetectorSummary,
	transactions []models.BrokerTransaction,
) error {
	ctx, span := s.tracer.Start(ctx, "state.DBStore.UpsertMarketDetector")
	defer span.End()

	logger := s.logger.With(slog.String("tag", "state.DBStore.UpsertMarketDetector"))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		err = fmt.Errorf("DBStore.UpsertMarketDetector begin tx: %w", err)
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
