package idx

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"time"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/blobstorage"
	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type AttachmentWorker interface {
	Work(ctx context.Context, task *AttachmentTask) (AttachmentResult, error)
}

func NewAttachmentWorker(
	configMinio *config.MinIO,
	logger *slog.Logger,
	tracer trace.Tracer,
	metrics *telemetry.Metrics,
	client Client,
	storage blobstorage.Storage,
) AttachmentWorker {
	return &attachment{
		configMinio: configMinio,
		logger:      logger,
		tracer:      tracer,
		metrics:     metrics,
		client:      client,
		storage:     storage,
		cleaner:     NewAttachmentPathCleaner(),
	}
}

type attachment struct {
	configMinio *config.MinIO
	logger      *slog.Logger
	tracer      trace.Tracer
	metrics     *telemetry.Metrics
	client      Client
	cleaner     AttachmentPathCleaner
	storage     blobstorage.Storage
}

func (a *attachment) Work(ctx context.Context, task *AttachmentTask) (AttachmentResult, error) {
	ctx, span := a.tracer.Start(
		ctx,
		"idx.attachment.Work",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	span.AddEvent("processing attachment work")

	start := time.Now()

	filePath := a.cleaner.Clean(ctx, *task)
	ctx = slogctx.Append(ctx, slog.String(constants.CleanFilepath, filePath))
	logger := a.logger.With(slog.String("tag", "idx.attachment.Work"))

	data, contentType, err := a.client.DownloadFile(ctx, task.Attachment.IdxURL)
	if err != nil {
		a.metrics.AttDownloadDuration.Record(ctx, float64(time.Since(start).Milliseconds()))
		a.metrics.AttDownloaded.Add(ctx, 1, metric.WithAttributes(
			attribute.String(constants.Status, "failure"),
		))
		task.Attachment.IsDownloaded = false
		task.Attachment.IsProcessing = false
		task.Attachment.Error = err.Error()
		return AttachmentResult{AttachmentTask: *task}, err
	}

	reader := bytes.NewReader(data)
	ctx = slogctx.Append(
		ctx,
		slog.Int(constants.Count, len(data)),
		slog.String(constants.ContentType, contentType),
	)
	result, err := a.storage.SaveReader(ctx, filePath, reader, int64(len(data)), contentType)
	if err != nil {
		a.metrics.AttDownloadDuration.Record(ctx, float64(time.Since(start).Milliseconds()))
		a.metrics.AttDownloadSize.Record(ctx, int64(len(data)))
		a.metrics.AttDownloaded.Add(ctx, 1, metric.WithAttributes(
			attribute.String(constants.Status, "failure"),
		))
		task.Attachment.IsDownloaded = false
		task.Attachment.IsProcessing = false
		task.Attachment.Error = err.Error()
		return AttachmentResult{AttachmentTask: *task}, err
	}
	ctx = slogctx.Append(ctx, slog.Any(constants.LocalSaveResult, result))

	a.metrics.AttDownloadDuration.Record(ctx, float64(time.Since(start).Milliseconds()))
	a.metrics.AttDownloadSize.Record(ctx, int64(len(data)))
	a.metrics.AttDownloaded.Add(ctx, 1, metric.WithAttributes(
		attribute.String(constants.Status, "success"),
	))

	task.Attachment.IsDownloaded = true
	task.Attachment.IsProcessing = false
	task.Attachment.Error = ""
	task.Attachment.Checksum = result.ChecksumSHA256
	task.Attachment.StoragePath = filepath.Clean(filePath)
	task.Attachment.UploadedAt = time.Now()
	logger.InfoContext(ctx, "attachment processed", slog.Any(constants.Attachment, task))
	span.AddEvent("attachment processed")
	return AttachmentResult{AttachmentTask: *task}, nil
}
