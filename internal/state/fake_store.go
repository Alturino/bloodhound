package state

import (
	"context"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	"github.com/alturino/bloodhound/internal/models"
)

// FakeStore implements the state.Store interface for testing
type FakeStore struct {
	Processed          map[string]bool
	SavedAnnouncements []models.Announcement
}

func (f *FakeStore) HasSavedAnnouncements(ctx context.Context) (bool, error) {
	return len(f.Processed) > 0, nil
}

func (f *FakeStore) IsProcessed(ctx context.Context, id string) (bool, error) {
	return f.Processed[id], nil
}

func (f *FakeStore) RecordAnnouncement(ctx context.Context, ann models.Announcement) error {
	if f.SavedAnnouncements == nil {
		f.SavedAnnouncements = []models.Announcement{}
	}
	if f.Processed == nil {
		f.Processed = make(map[string]bool)
	}
	f.SavedAnnouncements = append(f.SavedAnnouncements, ann)
	f.Processed[ann.ID2] = true
	return nil
}

func (f *FakeStore) RecordAttachment(
	ctx context.Context,
	idxID string,
	att models.Attachment,
	checksum string,
	storagePath string,
) error {
	return nil
}

func (f *FakeStore) UpsertMarketDetector(
	ctx context.Context,
	summary models.MarketDetectorSummary,
	transactions []models.BrokerTransaction,
) error {
	return nil
}

func (f *FakeStore) IsExists(ctx context.Context) (bool, error) {
	panic("not implemented") // TODO: Implement
}

// ShouldUpdate checks if the store need to be updated based on the latest announcement date
func (f *FakeStore) ShouldUpdate(ctx context.Context) (bool, error) {
	panic("not implemented") // TODO: Implement
}

func (f *FakeStore) LatestAnnouncement(ctx context.Context) (model.Announcements, error) {
	return model.Announcements{}, nil
}

func (f *FakeStore) GetStockCodesForMarketDetector(ctx context.Context) ([]string, error) {
	return []string{}, nil
}
