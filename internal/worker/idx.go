package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	slogcontext "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
	"github.com/alturino/bloodhound/pkg/idx"
)

type IDX struct {
	config           *config.Config
	logger           *slog.Logger
	announcementPool AnnouncementPool
	tracer           trace.Tracer
	client           idx.Client
	storage          storage.Storage
	store            state.IdxStore
}

func (w IDX) Start(ctx context.Context) error {
	interval := w.config.Scheduler.Interval

	logger := w.logger.With(
		slog.String("tag", "worker.WorkerIdx.Start"),
		slog.Duration("interval", interval),
	)

	if err := w.Process(ctx); err != nil {
		err = fmt.Errorf("initial processing: %w", err)
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return err
	}

	logger.InfoContext(ctx, "started background IDX worker")
	ticker := time.Tick(interval)
	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "stopping background IDX worker", slog.Any("error", ctx.Err()))
			return ctx.Err()
		case <-ticker:
			if err := w.Process(ctx); err != nil {
				return err
			}
		}
	}
}

func (w IDX) Process(ctx context.Context) error {
	ctx, span := w.tracer.Start(
		ctx,
		"worker.WorkerIdx.Process",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.WorkerIdx.Process"))

	isExists, err := w.store.IsExists(ctx)
	if err != nil {
		err = fmt.Errorf("is announcements exists: %w", err)
		return err
	}

	if !isExists {
		logger.InfoContext(ctx, "no existing announcements found, starting seeding")
		span.AddEvent("no existing announcements found, starting seeding")
		return w.processInitial(ctx)
	}

	logger.InfoContext(ctx, "announcements exists starting incremental sync")
	span.AddEvent("announcements exists starting incremental sync")
	return w.processIncremental(ctx)
}

func (w IDX) processInitial(ctx context.Context) error {
	return w.processAnnouncements(ctx, time.Time{}, "processInitial")
}

func (w IDX) processIncremental(ctx context.Context) error {
	latestAnnouncement, err := w.store.LatestAnnouncement(ctx)
	if err != nil {
		return err
	}
	return w.processAnnouncements(ctx, latestAnnouncement.Date, "processIncremental")
}

func (w IDX) processAnnouncements(ctx context.Context, since time.Time, tag string) error {
	ctx, span := w.tracer.Start(ctx, "worker.WorkerIdx."+tag)
	defer span.End()

	ctx = slogcontext.With(ctx, slog.String("tag", "worker.WorkerIdx."+tag))
	logger := w.logger

	resp, err := w.client.FetchAnnouncements(ctx, 0, since)
	if err != nil {
		err = fmt.Errorf("get total items and pages: %w", err)
		w.logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return err
	}
	totalItems, pageSize := resp.ResultCount, w.config.App.IDX.PageSize
	totalPages := (totalItems + pageSize) / pageSize

	workerCount := w.config.App.IDX.WorkerPool.AnnouncementWorkers
	ctx = slogcontext.With(ctx,
		slog.Int("total_items", totalItems),
		slog.Int("page_size", pageSize),
		slog.Int("total_pages", totalPages),
		slog.Int("worker_count", workerCount),
	)
	span.SetAttributes(
		attribute.Int("total_items", totalItems),
		attribute.Int("page_size", pageSize),
		attribute.Int("total_pages", totalPages),
		attribute.Int("worker_count", workerCount),
	)

	for page := totalPages - 1; page >= 0; page-- {
		ctx := slogcontext.With(ctx, slog.Int("page", page))

		resp, err := w.client.FetchAnnouncements(ctx, page, time.Time{})
		if err != nil {
			w.logger.ErrorContext(ctx, "fetch announcements", slog.Any("error", err))
			if w.config.App.Environment != "production" {
				return err
			}
			continue
		}
		if resp.ResultCount == 0 || len(resp.Replies) == 0 {
			w.logger.InfoContext(ctx, "page empty, stopping")
			break
		}

		announcements := make([]models.Announcement, len(resp.Replies))
		for i, reply := range resp.Replies {
			announcements[i] = reply.Announcement
		}

		results, err := w.announcementPool.ProcessPage(ctx, page, announcements)
		if err != nil {
			logger.ErrorContext(ctx, "process page", slog.Any("error", err))
		}
		if logger.Enabled(ctx, slog.LevelDebug) {
			logger.DebugContext(ctx, "processed", slog.Any("processed_page", results))
		}

		w.logger.InfoContext(ctx, "processing completed", slog.Int("page", page))
	}

	return nil
}

// func (w IDX) ProcessAttachment(
// 	ctx context.Context,
// 	ann models.Announcement,
// 	att models.Attachment,
// ) error {
// 	datePrefix := ann.Date.Format("2006-01-02")
// 	originalname := strings.ToLower(att.OriginalFilename)
// 	originalname = strings.ReplaceAll(originalname, ",", " ")
// 	originalname = strings.ReplaceAll(originalname, "//", " ")
// 	originalname = strings.ReplaceAll(originalname, " ", "_")
// 	filename := fmt.Sprintf("%s_%s", datePrefix, originalname)
// 	filePath := filepath.Join(
// 		strings.ToLower(ann.StockCode),
// 		fmt.Sprintf("%s_%s", datePrefix, strings.ToLower(ann.AnnouncementTitle)),
// 		filename,
// 	)
//
// 	bucket := w.config.MinIO.Bucket
// 	ctx, span := w.tracer.Start(
// 		ctx,
// 		"worker.WorkerIdx.processAttachment",
// 		trace.WithSpanKind(trace.SpanKindInternal),
// 		trace.WithAttributes(
// 			attribute.String("idx_filename", att.OriginalFilename),
// 			attribute.String("idx_attachment_url", att.FullSavePath),
// 			attribute.String("announcement_title", ann.AnnouncementTitle),
// 			attribute.String("bucket", bucket),
// 			attribute.String("filename", filePath),
// 		),
// 	)
// 	defer span.End()
//
// 	ctx = slogcontext.With(ctx,
// 		slog.String("tag", "worker.WorkerIdx.processAttachment"),
// 		slog.String("idx_filename", att.OriginalFilename),
// 		slog.String("idx_attachment_url", att.FullSavePath),
// 		slog.String("bucket", bucket),
// 		slog.String("filename", filePath),
// 	)
//
// 	data, contentType, err := w.client.DownloadFile(ctx, att.FullSavePath)
// 	if err != nil {
// 		err = fmt.Errorf("downloading attachment idx_attachment_url=%s : %w", att.FullSavePath, err)
// 		return err
// 	}
// 	checksum := calculateChecksum(data)
// 	if w.logger.Enabled(ctx, slog.LevelDebug) {
// 		ctx = slogcontext.With(ctx,
// 			slog.Int("size", len(data)),
// 			slog.String("content_type", contentType),
// 			slog.String("checksum_sha256", checksum),
// 		)
// 	}
//
// 	reader := bytes.NewReader(data)
// 	if err := w.storage.Upload(
// 		ctx,
// 		bucket,
// 		filePath,
// 		reader,
// 		int64(len(data)),
// 		contentType,
// 	); err != nil {
// 		err = fmt.Errorf("uploading attachment: %w", err)
// 		return err
// 	}
//
// 	if err := w.store.RecordAttachment(ctx, ann.ID2, att, checksum, filePath); err != nil {
// 		return err
// 	}
//
// 	return nil
// }

func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
