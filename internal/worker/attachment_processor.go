package worker

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	slogcontext "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
	"github.com/alturino/bloodhound/pkg/idx"
)

type AttachmentProcessor interface {
	ProcessAttachment(ctx context.Context, task AttachmentTask) error
}

func NewAttachmentProcessor(
	configMinio *config.MinIO,
	logger *slog.Logger,
	tracer trace.Tracer,
	client idx.Client,
	storage storage.Storage,
	store state.IdxStore,
) AttachmentProcessor {
	return &attachment{
		configMinio: configMinio,
		logger:      logger,
		tracer:      tracer,
		client:      client,
		storage:     storage,
		store:       store,
	}
}

type attachment struct {
	configMinio *config.MinIO
	logger      *slog.Logger
	tracer      trace.Tracer
	client      idx.Client
	storage     storage.Storage
	store       state.IdxStore
}

func (a attachment) ProcessAttachment(ctx context.Context, task AttachmentTask) error {
	datePrefix := task.Date.Format("2006-01-02")
	originalname := strings.ToLower(task.Attachment.OriginalFilename)
	originalname = strings.ReplaceAll(originalname, ",", " ")
	originalname = strings.ReplaceAll(originalname, "//", " ")
	originalname = strings.ReplaceAll(originalname, " ", "_")
	filename := fmt.Sprintf("%s_%s", datePrefix, originalname)

	stockCode := strings.ReplaceAll(task.StockCode, "..", "")
	stockCode = strings.ReplaceAll(stockCode, "/", "")
	announcementTitle := strings.ReplaceAll(task.AnnouncementTitle, "..", "")
	announcementTitle = strings.ReplaceAll(announcementTitle, "/", "")

	filePath := filepath.Join(
		strings.ToLower(stockCode),
		fmt.Sprintf("%s_%s", datePrefix, strings.ToLower(announcementTitle)),
		filename,
	)

	bucket := a.configMinio.Bucket
	ctx, span := a.tracer.Start(
		ctx,
		"worker.WorkerIdx.processAttachment",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("idx_filename", task.Attachment.OriginalFilename),
			attribute.String("idx_attachment_url", task.Attachment.FullSavePath),
			attribute.String("announcement_title", task.AnnouncementTitle),
			attribute.String("bucket", bucket),
			attribute.String("filename", filePath),
		),
	)
	defer span.End()

	ctx = slogcontext.With(ctx,
		slog.String("tag", "worker.WorkerIdx.processAttachment"),
		slog.String("idx_filename", task.Attachment.OriginalFilename),
		slog.String("idx_attachment_url", task.Attachment.FullSavePath),
		slog.String("bucket", bucket),
		slog.String("filename", filePath),
	)

	data, contentType, err := a.client.DownloadFile(ctx, task.Attachment.FullSavePath)
	if err != nil {
		err = fmt.Errorf(
			"downloading attachment idx_attachment_url=%s : %w",
			task.Attachment.FullSavePath,
			err,
		)
		return err
	}
	if a.logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogcontext.With(
			ctx,
			slog.Int("size", len(data)),
			slog.String("content_type", contentType),
		)
	}

	reader := bytes.NewReader(data)
	checksum, err := a.storage.Upload(ctx, bucket, filePath, reader, int64(len(data)), contentType)
	if err != nil {
		err = fmt.Errorf("uploading attachment: %w", err)
		return err
	}

	if err := a.store.RecordAttachment(
		ctx,
		task.AnnouncementID,
		task.Attachment,
		filePath,
		checksum,
	); err != nil {
		return err
	}

	return nil
}
