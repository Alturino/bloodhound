package idx

import (
	"bytes"
	"context"
	"log/slog"
	"time"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/blobstorage"
)

type AttachmentWorker interface {
	Work(ctx context.Context, task AttachmentTask) (AttachmentResult, error)
}

func NewAttachmentWorker(
	configMinio *config.MinIO,
	logger *slog.Logger,
	tracer trace.Tracer,
	client Client,
	storage blobstorage.Storage,
) AttachmentWorker {
	return &attachment{
		configMinio: configMinio,
		logger:      logger,
		tracer:      tracer,
		client:      client,
		storage:     storage,
		cleaner:     NewAttachmentPathCleaner(),
	}
}

type attachment struct {
	configMinio *config.MinIO
	logger      *slog.Logger
	tracer      trace.Tracer
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

	filePath := a.cleaner.Clean(ctx, task)
	ctx = slogctx.Append(ctx, slog.String("filepath", filePath))
	logger := a.logger.With(slog.String("tag", "attachment.Work"))

	data, contentType, err := a.client.DownloadFile(ctx, task.Attachment.FullSavePath)
	if err != nil {
		logger.ErrorContext(ctx, "downloading attachment", slog.Any("error", err))
		return AttachmentResult{
			IsDownloaded:   false,
			ChecksumSHA256: "",
			StoragePath:    "",
			Err:            err,
		}, err
	}

	reader := bytes.NewReader(data)
	ctx = slogctx.Append(ctx, slog.Int("size", len(data)), slog.String("content_type", contentType))
	result, err := a.storage.SaveReader(ctx, filePath, reader, int64(len(data)), contentType)
	if err != nil {
		logger.ErrorContext(ctx, "save attachment locally", slog.Any("error", err))
		return AttachmentResult{
			IsDownloaded:   false,
			ChecksumSHA256: "",
			StoragePath:    "",
			Err:            err,
		}, err
	}
	ctx = slogctx.Append(ctx, slog.Any("save_result", result))
	logger.InfoContext(ctx, "attachment processed")

	return AttachmentResult{
		IsDownloaded:   true,
		ChecksumSHA256: result.ChecksumSHA256,
		StoragePath:    result.Key,
		Err:            nil,
		UploadedAt:     time.Now(),
	}, nil
}
