package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
	"github.com/alturino/bloodhound/pkg/idx"
)

type WorkerIdx struct {
	config    *config.Config
	logger   *slog.Logger
	tracer   trace.Tracer
	idxClient idx.Client
	storage  storage.Storage
	idxStore state.IdxStore
}

func (w WorkerIdx) Start(ctx context.Context) error {
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

func (w WorkerIdx) Process(ctx context.Context) error {
	ctx, span := w.tracer.Start(
		ctx,
		"worker.WorkerIdx.Process",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.WorkerIdx.Process"))

	isExists, err := w.idxStore.IsExists(ctx)
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

func (w WorkerIdx) processInitial(ctx context.Context) error {
	ctx, span := w.tracer.Start(ctx, "worker.WorkerIdx.processInitial")
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.WorkerIdx.processInitial"))

	resp, err := w.idxClient.FetchAnnouncements(ctx, 0, time.Time{})
	if err != nil {
		err = fmt.Errorf("get total items and pages: %w", err)
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return err
	}
	totalItems, pageSize := resp.ResultCount, w.config.App.IDX.PageSize
	totalPages := (totalItems + pageSize) / pageSize
	logger = logger.With(
		slog.Int("total_items", totalItems),
		slog.Int("page_size", pageSize),
		slog.Int("total_pages", totalPages),
	)
	span.SetAttributes(
		attribute.Int("total_items", totalItems),
		attribute.Int("page_size", pageSize),
		attribute.Int("total_pages", totalPages),
	)

	processedCount := 0
	for page := totalPages - 1; page >= 0; page-- {
		logger := logger.With(slog.Int("page", page), slog.Int("processed_count", processedCount))

		resp, err := w.idxClient.FetchAnnouncements(ctx, page, time.Time{})
		if err != nil {
			logger.ErrorContext(ctx, "fetch announcements", slog.Any("error", err))
			if w.config.App.Environment != "production" {
				return err
			}
			continue
		}
		if resp.ResultCount == 0 || len(resp.Replies) == 0 {
			logger.InfoContext(ctx, "page empty, stopping")
			break
		}

		for announcement_i, reply := range resp.Replies {
			logger := logger.With(
				slog.Int("announcement_i", announcement_i),
				slog.Any("announcement", reply.Announcement),
			)
			if err := w.processAnnouncement(ctx, reply.Announcement); err != nil {
				logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
				if w.config.App.Environment != "production" {
					return err
				}
				continue
			}
			processedCount++
		}
	}

	return nil
}

func (w WorkerIdx) processIncremental(ctx context.Context) error {
	ctx, span := w.tracer.Start(ctx, "worker.WorkerIdx.processIncremental")
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.WorkerIdx.processIncremental"))

	latestAnnouncement, err := w.idxStore.LatestAnnouncement(ctx)
	if err != nil {
		return err
	}

	latestDate := latestAnnouncement.Date
	resp, err := w.idxClient.FetchAnnouncements(ctx, 0, latestDate)
	if err != nil {
		return err
	}
	totalItems, pageSize := resp.ResultCount, w.config.App.IDX.PageSize
	totalPages := (totalItems + pageSize) / pageSize
	logger = logger.With(
		slog.Int("total_items", totalItems),
		slog.Int("page_size", pageSize),
		slog.Int("total_pages", totalPages),
	)
	span.SetAttributes(
		attribute.Int("total_items", totalItems),
		attribute.Int("page_size", pageSize),
		attribute.Int("total_pages", totalPages),
	)

	for page := totalPages; page >= 0; page-- {
		logger := logger.With(slog.Int("page", page))

		resp, err := w.idxClient.FetchAnnouncements(ctx, page, latestDate)
		if err != nil {
			logger.ErrorContext(ctx, "fetch announcements", slog.Any("error", err))
			if w.config.App.Environment != "production" {
				return err
			}
			continue
		}
		if resp.ResultCount == 0 || len(resp.Replies) == 0 {
			logger.InfoContext(ctx, "page empty, stopping")
			break
		}

		for announcement_i, reply := range resp.Replies {
			logger := logger.With(
				slog.Int("announcement_i", announcement_i),
				slog.Any("announcement", reply.Announcement),
			)
			if err := w.processAnnouncement(ctx, reply.Announcement); err != nil {
				logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
				if w.config.App.Environment != "production" {
					return err
				}
				continue
			}
		}
	}

	return nil
}

func (w WorkerIdx) processAnnouncement(ctx context.Context, ann models.Announcement) error {
	ctx, span := w.tracer.Start(
		ctx,
		"worker.WorkerIdx.processAnnouncement",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("tag", "worker.WorkerIdx.processAnnouncement"),
			attribute.String("idx_announcement_id", ann.ID2),
			attribute.String("stock_code", ann.StockCode),
			attribute.String("announcement_date", ann.Date.String()),
		),
	)
	defer span.End()

	_ = w.logger.With(
		slog.String("tag", "worker.WorkerIdx.processAnnouncement"),
		slog.String("idx_announcement_id", ann.ID2),
		slog.String("stock_code", ann.StockCode),
		slog.Time("announcement_date", ann.Date),
	)

	if err := w.idxStore.RecordAnnouncement(ctx, ann); err != nil {
		err = fmt.Errorf("record announcement: %w", err)
		return err
	}

	var err error
	for _, att := range ann.Attachments {
		if processErr := w.processAttachment(ctx, ann, att); processErr != nil {
			processErr = fmt.Errorf("process attachment %s: %w", att.OriginalFilename, processErr)
			err = errors.Join(err, processErr)
		}
	}

	return err
}

func (w WorkerIdx) processAttachment(
	ctx context.Context,
	ann models.Announcement,
	att models.Attachment,
) error {
	datePrefix := ann.Date.Format("2006-01-02")
	originalname := strings.ToLower(att.OriginalFilename)
	originalname = strings.ReplaceAll(originalname, ",", " ")
	originalname = strings.ReplaceAll(originalname, "//", " ")
	originalname = strings.ReplaceAll(originalname, " ", "_")
	filename := fmt.Sprintf("%s_%s", datePrefix, originalname)
	filePath := filepath.Join(
		strings.ToLower(ann.StockCode),
		fmt.Sprintf("%s_%s", datePrefix, strings.ToLower(ann.AnnouncementTitle)),
		filename,
	)

	bucket := w.config.MinIO.Bucket
	ctx, span := w.tracer.Start(
		ctx,
		"worker.WorkerIdx.processAttachment",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("idx_filename", att.OriginalFilename),
			attribute.String("idx_attachment_url", att.FullSavePath),
			attribute.String("announcement_title", ann.AnnouncementTitle),
			attribute.String("bucket", bucket),
			attribute.String("filename", filePath),
		),
	)
	defer span.End()

	logger := w.logger.With(
		slog.String("tag", "worker.WorkerIdx.processAttachment"),
		slog.String("idx_filename", att.OriginalFilename),
		slog.String("idx_attachment_url", att.FullSavePath),
		slog.String("bucket", bucket),
		slog.String("filename", filePath),
	)

	data, contentType, err := w.idxClient.DownloadFile(ctx, att.FullSavePath)
	if err != nil {
		err = fmt.Errorf("downloading attachment idx_attachment_url=%s : %w", att.FullSavePath, err)
		return err
	}
	checksum := calculateChecksum(data)
	if logger.Enabled(ctx, slog.LevelDebug) {
		logger = logger.With(
			slog.Int("size", len(data)),
			slog.String("content_type", contentType),
			slog.String("checksum_sha256", checksum),
		)
	}

	reader := bytes.NewReader(data)
	if err := w.storage.Upload(
		ctx,
		bucket,
		filePath,
		reader,
		int64(len(data)),
		contentType,
	); err != nil {
		err = fmt.Errorf("uploading attachment: %w", err)
		return err
	}

	if err := w.idxStore.RecordAttachment(ctx, ann.ID2, att, checksum, filePath); err != nil {
		return err
	}

	return nil
}

func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}