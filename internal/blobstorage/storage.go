package blobstorage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/constants"
)

var ErrBucketExists = errors.New("bucket already exists")

// UploadInfo contains metadata about an uploaded object.
// This is a local type to avoid leaking MinIO-specific types through the Storage interface.
type UploadInfo struct {
	Bucket         string
	Key            string
	Location       string
	Size           int64
	ETag           string
	ChecksumSHA256 string
}

type SaveResult struct {
	UploadInfo
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
	// Upload uploads a file to the storage
	Upload(
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
	logger   *slog.Logger
	tracer   trace.Tracer
	storages []Storage
}

// NewStorage creates a multi-backend storage that fans out operations to all backends.
// Local file storage is always included. If MinIO is enabled in config, it is added as well.
func NewStorage(config *config.Storage, logger *slog.Logger, tracer trace.Tracer) (Storage, error) {
	logger.Debug("initializing local file storage")
	localLogger := logger.With(slog.String("tag", "blobstorage.localFile"))
	local := NewLocalFile(config.Local, localLogger, tracer)
	logger.Debug("initialized local file storage")

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

// Upload buffers the content using io.Copy and fans out the write
// to all backends in parallel. Returns the first backend's result.
func (s *storage) Upload(
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
		trace.WithAttributes(attribute.String(constants.File, filename)),
	)
	defer span.End()

	// Buffer content so each goroutine gets its own reader.
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, content); err != nil {
		return SaveResult{}, err
	}
	data := buf.Bytes()

	ctx = slogctx.Append(ctx, slog.String(constants.File, filename))
	logger := s.logger.With(slog.String("tag", "blobstorage.storage.Upload"))

	logger.DebugContext(ctx, "uploading file")
	span.AddEvent("uploading file")
	var wg sync.WaitGroup
	results := make([]SaveResult, len(s.storages))
	for i, backend := range s.storages {
		ctx := slogctx.Append(ctx, slog.Int("storages_index", i))
		wg.Go(func() {
			result, err := backend.Upload(
				ctx,
				filename,
				bytes.NewReader(data),
				contentSize,
				contentType,
			)
			if err != nil {
				logger.ErrorContext(ctx, "uploading file failed", slog.Any("error", err))
				return
			}
			results[i] = result
		})
	}
	wg.Wait()
	var res SaveResult
	for _, result := range results {
		if result.ChecksumSHA256 != "" {
			res = result
		}
	}
	logger.DebugContext(ctx, "uploaded file")
	span.AddEvent("uploaded file")

	return res, nil
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
	logger := s.logger.With(slog.String("tag", "blobstorage.storage.Exists"))

	logger.DebugContext(ctx, "checking file")
	span.AddEvent("checking file")
	var wg sync.WaitGroup
	results := make([]bool, len(s.storages))
	for i, backend := range s.storages {
		ctx := slogctx.Append(ctx, slog.Int("storages_index", i))
		wg.Go(func() {
			exists, err := backend.Exists(ctx, object)
			if err != nil {
				return
			}
			results[i] = exists
		})
	}
	wg.Wait()
	var isExists bool
	for _, result := range results {
		if result {
			isExists = result
			break
		}
	}
	logger.InfoContext(ctx, "checked file", slog.Bool(constants.IsExists, isExists))
	span.AddEvent(
		"checked file",
		trace.WithAttributes(attribute.Bool(constants.IsExists, isExists)),
	)

	return isExists, nil
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

	logger.DebugContext(ctx, "downloading")
	span.AddEvent("downloading")
	var reader io.ReadCloser
	for i, backend := range s.storages {
		ctx := slogctx.Append(ctx, slog.Int("storages_index", i))
		file, err := backend.Download(ctx, object)
		if err != nil {
			logger.ErrorContext(ctx, "downloading failed", slog.Any("error", err))
			continue
		}
		reader = file
	}
	if reader == nil {
		return nil, fmt.Errorf("object %s not found in any backend", object)
	}
	logger.InfoContext(ctx, "downloaded")
	span.AddEvent("downloaded")

	return reader, nil
}
