package state

import (
	"context"

	"github.com/alturino/bloodhound/internal/models"
)

// FakeStore implements the state.Store interface for testing
type FakeStore struct {
	Processed          map[string]bool
	SavedAnnouncements []models.Announcement
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

func (f *FakeStore) RecordAttachment(ctx context.Context, idxID string, att models.Attachment, checksum string, storagePath string) error {
	return nil
}
