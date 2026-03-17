package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOStorage implements Storage interface for MinIO
type MinIOStorage struct {
	client *minio.Client
	logger *slog.Logger
}

// NewMinIOStorage creates a new MinIO storage instance
func NewMinIOStorage(endpoint, accessKey, secretKey string, useSSL bool, logger *slog.Logger) (*MinIOStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("NewMinIOStorage: failed to create minio client: %w", err)
	}

	return &MinIOStorage{
		client: client,
		logger: logger,
	}, nil
}

// Upload uploads a file to MinIO
func (s *MinIOStorage) Upload(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, contentType string) error {
	_, err := s.client.PutObject(ctx, bucketName, objectName, reader, objectSize, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("MinIOStorage.Upload: failed to upload object %s to bucket %s: %w", objectName, bucketName, err)
	}

	s.logger.DebugContext(ctx, "successfully uploaded object",
		slog.String("bucket", bucketName),
		slog.String("object", objectName),
	)
	return nil
}

// Exists checks if an object exists in MinIO and is not empty
func (s *MinIOStorage) Exists(ctx context.Context, bucketName, objectName string) (bool, error) {
	info, err := s.client.StatObject(ctx, bucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("MinIOStorage.Exists: stat object %s in bucket %s: %w", objectName, bucketName, err)
	}
	return info.Size > 0, nil
// Exists checks if an object exists in MinIO
func (s *MinIOStorage) Exists(ctx context.Context, bucketName, objectName string) (bool, error) {
	_, err := s.client.StatObject(ctx, bucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("MinIOStorage.Exists: failed to stat object %s in bucket %s: %w", objectName, bucketName, err)
	}
	return true, nil
}

// Download downloads an object from MinIO
func (s *MinIOStorage) Download(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error) {
	object, err := s.client.GetObject(ctx, bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("MinIOStorage.Download: failed to get object %s from bucket %s: %w", objectName, bucketName, err)
	}
	return object, nil
}
