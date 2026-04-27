package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// MinIO implements Storage interface for MinIO
type MinIO struct {
	client *minio.Client
	logger *slog.Logger
	tracer trace.Tracer
}



// NewMinIO creates a new MinIO storage instance
func NewMinIO(
	config *config.MinIO,
	logger *slog.Logger,
	tracer trace.Tracer,
) (*MinIO, error) {
	if logger == nil {
		logger = slog.Default().With(slog.String("tag", "storage.MinIOStorage"))
	}
	if tracer == nil {
		tracer = telemetry.AppTelemetry.Tracer
	}
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("NewMinIOStorage create minio client: %w", err)
	}

	minio := &MinIO{client: client, logger: logger, tracer: tracer}
	if err := minio.CreateBucket(context.Background(), config.Bucket); err != nil {
		err = fmt.Errorf("create bucket: %w", err)
		return nil, err
	}

	return minio, nil
}

// Upload uploads a file to MinIO
func (s MinIO) Upload(
	ctx context.Context,
	bucket, filename string,
	contentReader io.Reader,
	objectSize int64,
	contentType string,
) (string, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"storage.MinIOStorage.Upload",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("bucket", bucket),
			attribute.String("file", filename),
		),
	)
	defer span.End()

	logger := s.logger.With(slog.String("bucket", bucket), slog.String("object", filename))

	logger.InfoContext(ctx, "uploading file")
	span.AddEvent("uploading file")
	info, err := s.client.PutObject(
		ctx,
		bucket,
		filename,
		contentReader,
		objectSize,
		minio.PutObjectOptions{
			ContentType:  contentType,
			AutoChecksum: minio.ChecksumSHA256,
		},
	)
	if err != nil {
		err = fmt.Errorf("upload file=%s bucket=%s: %w", filename, bucket, err)
		telemetry.RecordError(span, err)
		return "", err
	}
	logger.InfoContext(ctx, "uploaded file")
	span.AddEvent("uploaded file")

	return info.ChecksumSHA256, nil
}

// Exists checks if an object exists in MinIO and is not empty
func (s MinIO) Exists(ctx context.Context, bucket, object string) (bool, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"storage.MinIOStorage.Exists",
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
		err = fmt.Errorf("check object: %w", err)
		if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
			return false, nil
		}
		return false, err
	}
	logger.InfoContext(ctx, "object exists", slog.Int64("object_size", info.Size))
	span.AddEvent("object exists", trace.WithAttributes(attribute.Int64("object_size", info.Size)))

	return info.Size > 0, nil
}

// Download downloads a file from the storage
func (s MinIO) Download(
	ctx context.Context,
	bucket, object string,
) (io.ReadCloser, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"storage.MinIOStorage.Download",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("bucket", bucket),
			attribute.String("object", object),
		),
	)
	defer span.End()

	logger := s.logger.With(slog.String("bucket", bucket), slog.String("object", object))

	logger.DebugContext(ctx, "downloading object from MinIO")
	span.AddEvent("downloading object from MinIO")
	obj, err := s.client.GetObject(ctx, bucket, object, minio.GetObjectOptions{})
	if err != nil {
		err = fmt.Errorf("storage.MinIOStorage.Download download file: %w", err)
		return nil, err
	}
	logger.DebugContext(ctx, "downloaded object from MinIO")
	span.AddEvent("downloaded object from MinIO")

	return obj, nil
}

// CreateBucket creates a bucket if it doesn't exist
func (s MinIO) CreateBucket(ctx context.Context, bucket string) error {
	ctx, span := s.tracer.Start(
		ctx,
		"storage.MinIOStorage.CreateBucket",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("bucket", bucket)),
	)
	defer span.End()

	logger := s.logger.With(slog.String("bucket", bucket))

	logger.DebugContext(ctx, "is bucket exists")
	span.AddEvent("is bucket exists")
	isExists, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		err = fmt.Errorf("storage.MinIOStorage.CreateBucket is bucket exists: %w", err)
		return err
	}
	if !isExists {
		logger.DebugContext(ctx, "bucket doesn't exist, creating bucket")
		span.AddEvent("bucket doesn't exist, creating bucket")
		if err := s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			err = fmt.Errorf("storage.MinIOStorage.CreateBucket create bucket %w", err)
			return err
		}
	}
	logger.InfoContext(ctx, "bucket exists")
	span.AddEvent("bucket exists")

	return nil
}
