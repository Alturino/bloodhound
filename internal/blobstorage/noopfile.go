package blobstorage

import (
	"context"
	"io"
)

type noopLocalFile struct{}

// SaveReader uploads a file to the storage
func (n *noopLocalFile) SaveReader(
	ctx context.Context,
	filename string,
	content io.Reader,
	contentSize int64,
	contentType string,
) (SaveResult, error) {
	return SaveResult{}, nil
}

// Exists checks if a file exists in the storage
func (n *noopLocalFile) Exists(ctx context.Context, object string) (bool, error) {
	return false, nil
}

// Download downloads a file from the storage
func (n *noopLocalFile) Download(ctx context.Context, object string) (io.ReadCloser, error) {
	return nil, nil
}

// CreateBucket creates a bucket if it doesn't exist
func (n *noopLocalFile) CreateBucket(ctx context.Context) error {
	return nil
}
