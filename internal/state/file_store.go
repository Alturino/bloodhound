package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/alturino/bloodhound/internal/models"
)

// FileStore implements Store interface using a local JSON file
type FileStore struct {
	filePath string
	mu       sync.RWMutex
}

// State represents the persisted state
type State struct {
	LastProcessedID string `json:"last_processed_id"`
}

// NewFileStore creates a new file-based store
func NewFileStore(filePath string) (*FileStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf(
			"FileStore.NewFileStore: failed to create directory for state file: %w",
			err,
		)
	}

	return &FileStore{filePath: filePath}, nil
}

// GetLastProcessedID returns the last processed ID from the file
func (s *FileStore) GetLastProcessedID(ctx context.Context) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return "", nil
	}

	// TODO: use json streaming from the filepath instead readfile first then json unmarshal
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return "", fmt.Errorf("FileStore.GetLastProcessedID: failed to read state file: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return "", fmt.Errorf("FileStore.GetLastProcessedID: failed to parse state file: %w", err)
	}

	return state.LastProcessedID, nil
}

// SetLastProcessedID saves the last processed ID to the file
func (s *FileStore) SetLastProcessedID(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := State{LastProcessedID: id}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		err = fmt.Errorf("FileStore.SetLastProcessedID: failed to marshal state: %w", err)
return err
	}

	if err := os.WriteFile(s.filePath, data, 0o644); err != nil {
		err = fmt.Errorf("FileStore.SetLastProcessedID: failed to write state file: %w", err)
return err
	}

	return nil
}

// HasSavedAnnouncements checks if there are any processed announcements
func (s *FileStore) HasSavedAnnouncements(ctx context.Context) (bool, error) {
	lastID, err := s.GetLastProcessedID(ctx)
	if err != nil {
		return false, err
	}
	return lastID != "", nil
}

// IsProcessed checks if an announcement ID matches the last processed ID
func (s *FileStore) IsProcessed(ctx context.Context, id string) (bool, error) {
	lastID, err := s.GetLastProcessedID(ctx)
	if err != nil {
		return false, err
	}
	return id == lastID, nil
}

// RecordAnnouncement updates the last processed ID
func (s *FileStore) RecordAnnouncement(ctx context.Context, ann models.Announcement) error {
	return s.SetLastProcessedID(ctx, ann.ID2)
}

// RecordAttachment is a no-op for FileStore (simplified version)
func (s *FileStore) RecordAttachment(
	ctx context.Context,
	annID string,
	att models.Attachment,
	checksum string,
	storagePath string,
) error {
	return nil
}
