package state

import (
	"context"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	"github.com/alturino/bloodhound/internal/models"
)

// Store defines the interface for state persistence
type Store interface {
	IsExists(ctx context.Context) (bool, error)
	// LatestAnnouncement checks if the store need to be updated based on the latest announcement date
	LatestAnnouncement(ctx context.Context) (model.Announcements, error)
	// IsProcessed checks if an announcement has already been processed
	IsProcessed(ctx context.Context, id string) (bool, error)
	// RecordAnnouncement saves announcement metadata to the database
	RecordAnnouncement(ctx context.Context, ann models.Announcement) error
	// RecordAttachment saves attachment metadata to the database
	RecordAttachment(
		ctx context.Context,
		annID string,
		att models.Attachment,
		checksum string,
		storagePath string,
	) error
	// UpsertMarketDetector inserts or updates market detector summaries and transactions
	UpsertMarketDetector(
		ctx context.Context,
		summary models.MarketDetectorSummary,
		transactions []models.BrokerTransaction,
	) error
}
