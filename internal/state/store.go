package state

import (
	"context"

	"github.com/alturino/bloodhound/internal/models"
)

// Store defines the interface for state persistence
type Store interface {
	// HasSavedAnnouncements checks if the store contains any processed announcements
	HasSavedAnnouncements(ctx context.Context) (bool, error)
	// IsProcessed checks if an announcement has already been processed
	IsProcessed(ctx context.Context, id string) (bool, error)
	// RecordAnnouncement saves announcement metadata to the database
	RecordAnnouncement(ctx context.Context, ann models.Announcement) error
	// RecordAttachment saves attachment metadata to the database
	RecordAttachment(ctx context.Context, annID string, att models.Attachment, checksum string, storagePath string) error
}
