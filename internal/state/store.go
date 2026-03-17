package state

import (
	"context"
)

// Store defines the interface for state persistence
type Store interface {
	// GetLastProcessedID returns the ID of the last processed announcement
	GetLastProcessedID(ctx context.Context) (string, error)
	// SetLastProcessedID sets the ID of the last processed announcement
	SetLastProcessedID(ctx context.Context, id string) error
}
