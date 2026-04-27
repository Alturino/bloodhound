package storage

import (
	"context"
	"errors"
	"io"
)

var ErrBucketExists = errors.New("bucket already exists")

// Storage defines the interface for file storage operations
type Storage interface {
	// Upload uploads a file to the storage
	Upload(
		ctx context.Context,
		bucket, filename string,
		contentReader io.Reader,
		objectSize int64,
		contentType string,
	) (string, error)
	// Exists checks if a file exists in the storage
	Exists(ctx context.Context, bucketName, objectName string) (bool, error)
	// Download downloads a file from the storage
	Download(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error)
	// CreateBucket creates a bucket if it doesn't exist
	CreateBucket(ctx context.Context, bucketName string) error
}
