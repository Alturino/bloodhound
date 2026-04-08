package storage

import (
	"context"
	"io"
)

// Storage defines the interface for file storage operations
type Storage interface {
	// Upload uploads a file to the storage
	Upload(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, contentType string) error
	// Exists checks if a file exists in the storage
	Exists(ctx context.Context, bucketName, objectName string) (bool, error)
	// Download downloads a file from the storage
	Download(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error)
}
