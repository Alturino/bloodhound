package state

import (
	"context"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	"github.com/alturino/bloodhound/internal/models"
)

// IdxStore - IDX announcement state persistence
type IdxStore interface {
	IsExists(ctx context.Context) (bool, error)
	LatestAnnouncement(ctx context.Context) (model.Announcements, error)
	IsProcessed(ctx context.Context, id string) (bool, error)
	RecordAnnouncement(ctx context.Context, ann models.Announcement) error
	RecordAttachment(ctx context.Context, annID string, att models.Attachment, checksum string, storagePath string) error
}

// StockbitStore - Stockbit market detector state persistence
type StockbitStore interface {
	UpsertMarketDetector(ctx context.Context, summary models.MarketDetectorSummary, transactions []models.BrokerTransaction) error
	GetStockCodesForMarketDetector(ctx context.Context) ([]string, error)
}