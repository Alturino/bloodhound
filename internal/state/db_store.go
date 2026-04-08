package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	. "github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"

	. "github.com/alturino/bloodhound/internal/db/.gen/postgres/public/table"
	"github.com/alturino/bloodhound/internal/db/.gen/postgres/public/model"
	"github.com/alturino/bloodhound/internal/models"
)

// DBStore implements the Store interface using PostgreSQL and go-jet
type DBStore struct {
	db *sql.DB
}

func (s *DBStore) HasSavedAnnouncements(ctx context.Context) (bool, error) {
	var ann models.Announcement
	stmt := SELECT(Announcements.AllColumns).FROM(Announcements).LIMIT(1)
	err := stmt.QueryContext(ctx, s.db, &ann)
	if err != nil {
		if errors.Is(err, qrm.ErrNoRows){
			return false, nil
		}
		return false, fmt.Errorf("DBStore.HasSavedAnnouncements: %w", err)
	}
	return true, nil
}

// NewDBStore creates a new PostgreSQL-backed store
func NewDBStore(db *sql.DB) *DBStore {
	return &DBStore{db: db}
}

// IsProcessed checks if an announcement ID exists in the database by its IDX ID
func (s *DBStore) IsProcessed(ctx context.Context, idxID string) (bool, error) {
	var isExist bool
	isExistStmt := SELECT(Int(1)).FROM(Announcements).WHERE(Announcements.IdxID.EQ(String(idxID)))
	if err := SELECT(EXISTS(isExistStmt)).QueryContext(ctx, s.db, &isExist); err != nil {
		return false, fmt.Errorf("DBStore.IsProcessed: %w", err)
	}
	return isExist, nil
}

// RecordAnnouncement saves announcement metadata
func (s *DBStore) RecordAnnouncement(ctx context.Context, ann models.Announcement) error {
	announcement := ann.ToAnnouncements()
	if err := Announcements.INSERT(Announcements.AllColumns.Except(Announcements.ID, Announcements.CreatedAt)).
		MODEL(announcement).
		RETURNING(Announcements.AllColumns).
		QueryContext(ctx, s.db, &announcement); err != nil {
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
	attachment := att.ToAttachments(idxID, checksum, storagePath)
	if err := Attachments.INSERT(Attachments.AllColumns.Except(Attachments.ID)).
		MODEL(attachment).
		RETURNING(Attachments.AllColumns).
		QueryContext(ctx, s.db, &attachment); err != nil {
		return fmt.Errorf("DBStore.RecordAttachment: %w", err)
	}
	return nil
}

// UpsertMarketDetector inserts or updates market detector summaries and transactions
func (s *DBStore) UpsertMarketDetector(ctx context.Context, summary models.MarketDetectorSummary, transactions []models.BrokerTransaction) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("DBStore.UpsertMarketDetector: begin tx: %w", err)
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
		MarketDetectorSummaries.Symbol,
		MarketDetectorSummaries.TradeDate,
		MarketDetectorSummaries.AccdistStatus,
		MarketDetectorSummaries.TotalValue,
	).
		MODEL(sumModel).
		ON_CONFLICT(MarketDetectorSummaries.Symbol, MarketDetectorSummaries.TradeDate).
		DO_UPDATE(SET(
			MarketDetectorSummaries.AccdistStatus.SET(MarketDetectorSummaries.EXCLUDED.AccdistStatus),
			MarketDetectorSummaries.TotalValue.SET(MarketDetectorSummaries.EXCLUDED.TotalValue),
		)).
		RETURNING(MarketDetectorSummaries.ID)

	if err := stmt.QueryContext(ctx, tx, &dbSummary); err != nil {
		return fmt.Errorf("upsert summary: %w", err)
	}

	if len(transactions) > 0 {
		var dbTxns []model.BrokerTransactions
		for _, t := range transactions {
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
		}

		insStmt := BrokerTransactions.INSERT(
			BrokerTransactions.SummaryID,
			BrokerTransactions.Symbol,
			BrokerTransactions.TradeDate,
			BrokerTransactions.BrokerCode,
			BrokerTransactions.Side,
			BrokerTransactions.AvgPrice,
			BrokerTransactions.Lots,
			BrokerTransactions.InvestorType,
			BrokerTransactions.Frequency,
		).MODELS(dbTxns).
			ON_CONFLICT(BrokerTransactions.Symbol, BrokerTransactions.TradeDate, BrokerTransactions.BrokerCode, BrokerTransactions.Side).
			DO_UPDATE(SET(
				BrokerTransactions.AvgPrice.SET(BrokerTransactions.EXCLUDED.AvgPrice),
				BrokerTransactions.Lots.SET(BrokerTransactions.EXCLUDED.Lots),
				BrokerTransactions.Frequency.SET(BrokerTransactions.EXCLUDED.Frequency),
			))

		if _, err := insStmt.ExecContext(ctx, tx); err != nil {
			return fmt.Errorf("upsert broker transactions: %w", err)
		}
	}

	return tx.Commit()
}
