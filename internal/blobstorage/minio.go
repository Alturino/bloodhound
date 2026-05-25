package blobstorage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"path/filepath"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// MinIO implements Storage interface for MinIO
type MinIO struct {
	config  *config.MinIO
	client  *minio.Client
	storage Storage
	logger  *slog.Logger
	tracer  trace.Tracer
}

// NewMinIO creates a new MinIO storage instance
func NewMinIO(
	config *config.MinIO,
	storage Storage,
	logger *slog.Logger,
	tracer trace.Tracer,
) (*MinIO, error) {
	if logger == nil {
		logger = slog.Default().With(slog.String("tag", "blobstorage.MinIO"))
	}
	if tracer == nil {
		tracer = telemetry.AppTelemetry.Tracer
	}
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:           credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure:          config.UseSSL,
		TrailingHeaders: true,
	})
	if err != nil {
		return nil, fmt.Errorf("NewMinIOStorage create minio client: %v", err)
	}

	minio := &MinIO{
		client:  client,
		storage: storage,
		logger:  logger,
		tracer:  tracer,
		config:  config,
	}
	if err := minio.CreateBucket(context.Background()); err != nil {
		err = fmt.Errorf("create bucket: %v", err)
		return nil, err
	}

	return minio, nil
}

// Upload uploads a file to MinIO
func (s *MinIO) SaveReader(
	ctx context.Context,
	filename string,
	content io.Reader,
	contentSize int64,
	contentType string,
) (SaveResult, error) {
	bucket := s.config.Bucket
	ctx, span := s.tracer.Start(
		ctx,
		"blobstorage.MinIO.SaveReader",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("bucket", bucket),
			attribute.String("file", filename),
		),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "blobstorage.MinIO.SaveReader"),
		slog.String("bucket", bucket),
		slog.String("object", filename),
	)

	logger.DebugContext(ctx, "uploading file")
	span.AddEvent("uploading file")
	var buf bytes.Buffer
	tee := io.TeeReader(content, &buf)
	info, err := s.client.PutObject(
		ctx,
		bucket,
		filename,
		tee,
		contentSize,
		minio.PutObjectOptions{
			AutoChecksum: minio.ChecksumSHA256,
			Checksum:     minio.ChecksumSHA256,
			ContentType:  mime.TypeByExtension(filepath.Ext(filename)),
		},
	)
	if err != nil {
		err = fmt.Errorf("saving file: %v", err)
		telemetry.RecordError(span, err)
		return SaveResult{}, err
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.Any("minio_result", info))
	}
	logger.InfoContext(ctx, "saved file")
	span.AddEvent("saved file")

	saveres, err := s.storage.SaveReader(ctx, filename, &buf, contentSize, contentType)
	if err != nil {
		logger.WarnContext(ctx, "saving file locally", slog.Any("error", err))
		return SaveResult{UploadInfo: info}, nil
	}
	ctx = slogctx.Append(ctx, slog.Any("local_save_res", saveres))
	logger.InfoContext(ctx, "saved file locally")

	return SaveResult{UploadInfo: info}, nil
}

// Exists checks if an object exists in MinIO and is not empty
func (s *MinIO) Exists(ctx context.Context, object string) (bool, error) {
	bucket := s.config.Bucket
	ctx, span := s.tracer.Start(
		ctx,
		"blobstorage.MinIO.Exists",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("bucket", bucket),
			attribute.String("object", object),
		),
	)
	defer span.End()

	logger := s.logger.With(slog.String("bucket", bucket), slog.String("object", object))

	logger.DebugContext(ctx, "check object")
	span.AddEvent("check object")
	info, err := s.client.StatObject(ctx, bucket, object, minio.StatObjectOptions{})
	if err != nil {
		err = fmt.Errorf("check object: %v", err)
		if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
			return false, nil
		}
		telemetry.RecordError(span, err)
		return false, err
	}
	logger.InfoContext(ctx, "object exists", slog.Int64("object_size", info.Size))
	span.AddEvent("object exists", trace.WithAttributes(attribute.Int64("object_size", info.Size)))

	return info.Size > 0, nil
}

// Download downloads a file from the storage
func (s *MinIO) Download(ctx context.Context, object string) (io.ReadCloser, error) {
	bucket := s.config.Bucket
	ctx, span := s.tracer.Start(
		ctx,
		"blobstorage.MinIO.Download",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("bucket", bucket),
			attribute.String("object", object),
		),
	)
	defer span.End()

	logger := s.logger.With(slog.String("bucket", bucket), slog.String("object", object))

	logger.DebugContext(ctx, "downloading object from minio")
	span.AddEvent("downloading object from minio")
	obj, err := s.client.GetObject(ctx, bucket, object, minio.GetObjectOptions{})
	if err != nil {
		err = fmt.Errorf("downloading object from minio: %w", err)
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		telemetry.RecordError(span, err)
		return nil, err
	}
	logger.DebugContext(ctx, "downloaded object from minio")
	span.AddEvent("downloaded object from minio")

	return obj, nil
}

// CreateBucket creates a bucket if it doesn't exist
func (s *MinIO) CreateBucket(ctx context.Context) error {
	bucket := s.config.Bucket
	ctx, span := s.tracer.Start(
		ctx,
		"blobstorage.MinIO.CreateBucket",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("bucket", bucket)),
	)
	defer span.End()

	logger := s.logger.With(slog.String("bucket", bucket))

	logger.DebugContext(ctx, "is bucket exists")
	span.AddEvent("is bucket exists")
	isExists, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		err = fmt.Errorf("minio is bucket exists: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	if !isExists {
		logger.DebugContext(ctx, "bucket doesn't exist, creating bucket")
		span.AddEvent("bucket doesn't exist, creating bucket")
		if err := s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			err = fmt.Errorf("minio create bucket: %w", err)
			telemetry.RecordError(span, err)
			return err
		}
	}
	logger.InfoContext(ctx, "bucket exists")
	span.AddEvent("bucket exists")

	return nil
}
