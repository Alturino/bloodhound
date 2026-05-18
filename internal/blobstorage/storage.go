package blobstorage

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/minio/minio-go/v7"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
)

var ErrBucketExists = errors.New("bucket already exists")

type SaveResult struct {
	minio.UploadInfo
}

func (s SaveResult) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("bucket", s.Bucket),
		slog.Int64("size", s.Size),
		slog.String("key", s.Location),
		slog.String("etag", s.ETag),
		slog.String("checksum_sha256", s.ChecksumSHA256),
	)
}

// Storage defines the interface for file storage operations
type Storage interface {
	// SaveReader uploads a file to the storage
	SaveReader(
		ctx context.Context,
		filename string,
		content io.Reader,
		contentSize int64,
		contentType string,
	) (SaveResult, error)
	// Exists checks if a file exists in the storage
	Exists(ctx context.Context, object string) (bool, error)
	// Download downloads a file from the storage
	Download(ctx context.Context, object string) (io.ReadCloser, error)
	// CreateBucket creates a bucket if it doesn't exist
	CreateBucket(ctx context.Context) error
}

type storage struct {
	s      Storage
	logger *slog.Logger
	tracer trace.Tracer
}

func NewStorage(config *config.Storage, logger *slog.Logger, tracer trace.Tracer) (Storage, error) {
	fsLogger := logger.With(slog.String("tag", "storage.Filestorage"))
	fs := NewLocalFile(&config.Local, fsLogger, tracer)

	minioLogger := logger.With(slog.String("tag", "storage.Minio"))
	minio, err := NewMinIO(&config.MinIO, fs, minioLogger, tracer)
	if err != nil {
		logger.Error("create minio client", slog.Any("error", err))
		return nil, err
	}

	return &storage{s: minio, logger: logger, tracer: tracer}, nil
}

// SaveReader uploads a file to the storage
func (s *storage) SaveReader(
	ctx context.Context,
	filename string,
	content io.Reader,
	contentSize int64,
	contentType string,
) (SaveResult, error) {
	logger := s.logger.With(slog.String("tag", "storage.Storage.SaveReader"))
	logger.DebugContext(ctx, "saving file")
	return s.s.SaveReader(ctx, filename, content, contentSize, contentType)
}

// Exists checks if a file exists in the storage
func (s *storage) Exists(ctx context.Context, object string) (bool, error) {
	return s.s.Exists(ctx, object)
}

// Download downloads a file from the storage
func (s *storage) Download(ctx context.Context, object string) (io.ReadCloser, error) {
	return s.s.Download(ctx, object)
}

// CreateBucket creates a bucket if it doesn't exist
func (s *storage) CreateBucket(ctx context.Context) error {
	return s.s.CreateBucket(ctx)
}
