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

	return &MinIO{
		client: client,
		logger: logger,
		tracer: tracer,
	}, nil
}

// Upload uploads a file to MinIO
func (s *MinIO) Upload(
	ctx context.Context,
	bucketName, objectName string,
	reader io.Reader,
	objectSize int64,
	contentType string,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"storage.MinIOStorage.Upload",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("bucket", bucketName),
			attribute.String("object", objectName),
		),
	)
	defer span.End()

	logger := s.logger.With(slog.String("bucket", bucketName), slog.String("object", objectName))

	logger.DebugContext(ctx, "uploading object to MinIO")
	span.AddEvent("uploading object to MinIO")
	_, err := s.client.PutObject(
		ctx,
		bucketName,
		objectName,
		reader,
		objectSize,
		minio.PutObjectOptions{
			ContentType: contentType,
		},
	)
	if err != nil {
		err = fmt.Errorf(
			"MinIOStorage.Upload upload object %s to bucket %s: %w",
			objectName,
			bucketName,
			err,
		)
		telemetry.RecordError(span, err)
		return err
	}

	logger.InfoContext(ctx, "successfully uploaded object")
	span.AddEvent("successfully uploaded object")
	return nil
}

// Exists checks if an object exists in MinIO and is not empty
func (s *MinIO) Exists(ctx context.Context, bucket, object string) (bool, error) {
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

	logger.DebugContext(ctx, "checking if object exists in MinIO")
	span.AddEvent("checking if object exists in MinIO")
	info, err := s.client.StatObject(ctx, bucket, object, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		err = fmt.Errorf(
			"storage.MinIOStorage.Exists object %s in bucket %s: %w",
			object,
			bucket,
			err,
		)
		return false, err
	}
	logger.InfoContext(ctx, "object is exists")
	span.AddEvent("object is exists")

	return info.Size > 0, nil
}

// Download downloads a file from the storage
func (s *MinIO) Download(
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
func (s *MinIO) CreateBucket(ctx context.Context, bucketName string) error {
	ctx, span := s.tracer.Start(
		ctx,
		"storage.MinIOStorage.CreateBucket",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("bucket", bucketName)),
	)
	defer span.End()

	logger := s.logger.With(slog.String("bucket", bucketName))

	logger.DebugContext(ctx, "creating bucket in MinIO")
	span.AddEvent("creating bucket in MinIO")

	err := s.client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "BucketAlreadyExists" {
			return ErrBucketExists
		}
		err = fmt.Errorf("storage.MinIOStorage.CreateBucket create bucket %s: %w", bucketName, err)
		return err
	}

	logger.InfoContext(ctx, "successfully created bucket")
	span.AddEvent("successfully created bucket")
	return nil
}
