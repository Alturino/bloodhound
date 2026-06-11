package blobstorage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/minio/minio-go/v7"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

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
		slog.String("key", s.Key),
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

// storage fans out operations to multiple Storage backends in parallel.
type storage struct {
	storages []Storage
	logger   *slog.Logger
	tracer   trace.Tracer
}

// NewStorage creates a multi-backend storage that fans out operations to all backends.
// Local file storage is always included. If MinIO is enabled in config, it is added as well.
func NewStorage(config *config.Storage, logger *slog.Logger, tracer trace.Tracer) (Storage, error) {
	logger.Debug("initializing local file storage storage")
	localLogger := logger.With(slog.String("tag", "blobstorage.localFile"))
	local := NewLocalFile(config.Local, localLogger, tracer)
	logger.Info("initialized local file storage storage")

	logger.Debug("initializing minio storage")
	minioLogger := logger.With(slog.String("tag", "blobstorage.Minio"))
	minio, err := NewMinIO(config.MinIO, minioLogger, tracer)
	if err != nil {
		return nil, err
	}
	logger.Info("initialized minio storage")

	backends := []Storage{local, minio}
	return &storage{storages: backends, logger: logger, tracer: tracer}, nil
}

// SaveReader buffers the content using io.Copy and fans out the write
// to all backends in parallel. Returns the first backend's result.
func (s *storage) SaveReader(
	ctx context.Context,
	filename string,
	content io.Reader,
	contentSize int64,
	contentType string,
) (SaveResult, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"blobstorage.storage.SaveReader",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(),
	)
	defer span.End()

	// Buffer content so each goroutine gets its own reader.
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, content); err != nil {
		return SaveResult{}, err
	}
	data := buf.Bytes()

	logger := s.logger.With(slog.String("tag", "blobstorage.storage.SaveReader"))

	var wg sync.WaitGroup
	results := make([]SaveResult, len(s.storages))
	for i, backend := range s.storages {
		ctx := slogctx.Append(ctx, slog.Int("storages_index", i))
		wg.Go(func() {
			result, err := backend.SaveReader(
				ctx,
				filename,
				bytes.NewReader(data),
				contentSize,
				contentType,
			)
			if err != nil {
				logger.WarnContext(ctx, "save failed", slog.Any("error", err))
				return
			}
			results[i] = result
		})
	}
	wg.Wait()

	return results[0], nil
}

// CreateBucket fans out bucket creation to all backends in parallel.
func (s *storage) CreateBucket(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	for i, backend := range s.storages {
		g.Go(func() error {
			if err := backend.CreateBucket(ctx); err != nil {
				s.logger.Error(
					"backend CreateBucket failed",
					slog.Any("error", err),
					slog.Int("backend_index", i),
				)
				return err
			}
			return nil
		})
	}
	return g.Wait()
}

// Exists returns true if any backend reports the object exists.
func (s *storage) Exists(ctx context.Context, object string) (bool, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"blobstorage.storage.Exists",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(),
	)
	defer span.End()

	ctx = slogctx.Append(ctx, slog.String("object_key", object))

	g, ctx := errgroup.WithContext(ctx)
	results := make([]bool, len(s.storages))
	for i, backend := range s.storages {
		ctx := slogctx.Append(ctx, slog.Int("storages_index", i))
		g.Go(func() error {
			exists, err := backend.Exists(ctx, object)
			if err != nil {
				return err
			}
			results[i] = exists
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return false, err
	}

	for _, result := range results {
		if result {
			return true, nil
		}
	}
	return false, nil
}

// Download retrieves the object from the first available backend.
func (s *storage) Download(ctx context.Context, object string) (io.ReadCloser, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"blobstorage.storage.Download",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "blobstorage.storage.Download"))

	for i, backend := range s.storages {
		ctx := slogctx.Append(ctx, slog.Int("storages_index", i))
		reader, err := backend.Download(ctx, object)
		if err != nil {
			logger.ErrorContext(ctx, "storage failed", slog.Any("error", err))
			continue
		}
		if reader != nil {
			return reader, nil
		}
	}
	return nil, fmt.Errorf("object %s not found in any backend", object)
}
