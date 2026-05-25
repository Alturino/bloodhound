package idx

import (
	"bytes"
	"context"
	"log/slog"
	"time"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/blobstorage"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type AttachmentWorker interface {
	Work(ctx context.Context, task AttachmentTask) (AttachmentResult, error)
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

func (a *attachment) Work(ctx context.Context, task AttachmentTask) (AttachmentResult, error) {
	ctx, span := a.tracer.Start(
		ctx,
		"idx.attachment.Work",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	start := time.Now()

	filePath := a.cleaner.Clean(ctx, task)
	ctx = slogctx.Append(ctx, slog.String("filepath", filePath))
	logger := a.logger.With(slog.String("tag", "attachment.Work"))

	data, contentType, err := a.client.DownloadFile(ctx, task.Attachment.IdxURL)
	if err != nil {
		a.metrics.AttDownloadDuration.Record(ctx, float64(time.Since(start).Milliseconds()))
		a.metrics.AttDownloaded.Add(ctx, 1, metric.WithAttributes(
			attribute.String("status", "failure"),
		))
		logger.ErrorContext(ctx, "downloading attachment", slog.Any("error", err))
		task.Attachment.IsDownloaded = false
		task.Attachment.IsProcessing = false
		task.Attachment.Error = err.Error()
		return AttachmentResult{AttachmentTask: task}, err
	}

	reader := bytes.NewReader(data)
	ctx = slogctx.Append(ctx, slog.Int("size", len(data)), slog.String("content_type", contentType))
	result, err := a.storage.SaveReader(ctx, filePath, reader, int64(len(data)), contentType)
	if err != nil {
		a.metrics.AttDownloadDuration.Record(ctx, float64(time.Since(start).Milliseconds()))
		a.metrics.AttDownloadSize.Record(ctx, int64(len(data)))
		a.metrics.AttDownloaded.Add(ctx, 1, metric.WithAttributes(
			attribute.String("status", "failure"),
		))
		logger.ErrorContext(ctx, "save attachment locally", slog.Any("error", err))
		task.Attachment.IsDownloaded = false
		task.Attachment.IsProcessing = false
		task.Attachment.Error = err.Error()
		return AttachmentResult{AttachmentTask: task}, err
	}
	ctx = slogctx.Append(ctx, slog.Any("save_result", result))
	logger.InfoContext(ctx, "attachment processed")

	a.metrics.AttDownloadDuration.Record(ctx, float64(time.Since(start).Milliseconds()))
	a.metrics.AttDownloadSize.Record(ctx, int64(len(data)))
	a.metrics.AttDownloaded.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", "success"),
	))

	task.Attachment.IsDownloaded = true
	task.Attachment.IsProcessing = false
	task.Attachment.Error = ""
	return AttachmentResult{AttachmentTask: task}, nil
}
