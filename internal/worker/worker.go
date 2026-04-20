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
	"regexp"
	"strings"
	"time"

	slogcontext "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/idx"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/stockbit"
	"github.com/alturino/bloodhound/internal/storage"
)

// Worker handles the orchestration of fetching and processing announcements
type Worker struct {
	config         *config.Config
	logger         *slog.Logger
	idxClient      idx.Client
	stockbitClient stockbit.Client
	storage        storage.Storage
	stateStore     state.Store
	tracer         trace.Tracer
}

// NewWorker initializes a new background worker
func NewWorker(
	idxClient idx.Client,
	stockbitClient stockbit.Client,
	storage storage.Storage,
	stateStore state.Store,
	cfg *config.Config,
	logger *slog.Logger,
	tracer trace.Tracer,
) *Worker {
	return &Worker{
		idxClient:      idxClient,
		stockbitClient: stockbitClient,
		storage:        storage,
		stateStore:     stateStore,
		config:         cfg,
		logger:         logger,
		tracer:         tracer,
	}
}

// Start starts the background worker loop
func (w Worker) Start(ctx context.Context) error {
	interval := w.config.Scheduler.Interval

	logger := w.logger.With(
		slog.String("tag", "worker.Worker.Start"),
		slog.Duration("interval", interval),
	)

	// Run once immediately
	if err := w.Process(ctx); err != nil {
		err = fmt.Errorf("initial processing: %w", err)
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return err
	}

	logger.InfoContext(ctx, "started background worker")
	ticker := time.Tick(interval)
	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "stopping background worker", slog.Any("error", ctx.Err()))
			return ctx.Err()
		case <-ticker:
			if err := w.Process(ctx); err != nil {
				return err
			}
		}
	}
}

// Process executes one cycle of polling and processing
func (w Worker) Process(ctx context.Context) error {
	ctx, span := w.tracer.Start(
		ctx,
		"worker.Worker.Process",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.Worker.Process"))

	isExists, err := w.stateStore.IsExists(ctx)
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

// processInitial performs a full scan from last page to first to seed the database
func (w Worker) processInitial(ctx context.Context) error {
	ctx, span := w.tracer.Start(ctx, "worker.Worker.processInitial")
	defer span.End()

	logger := w.logger.With()
	ctx = slogcontext.Append(ctx, slog.String("tag", "worker.Worker.processInitial"))

	resp, err := w.idxClient.FetchAnnouncements(ctx, 0, time.Time{})
	if err != nil {
		err = fmt.Errorf("get total items and pages: %w", err)
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return err
	}
	totalItems, pageSize := resp.ResultCount, w.config.App.IDX.PageSize
	totalPages := (totalItems + pageSize) / pageSize
	ctx = slogcontext.Append(ctx,
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
		ctx := slogcontext.Append(
			ctx,
			slog.Int("page", page),
			slog.Int("processed_count", processedCount),
		)

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

		for _, reply := range resp.Replies {
			ctx := slogcontext.Append(ctx,
				slog.Int("page", page),
				slog.Int("processed_count", processedCount),
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

// processIncremental polls page 1 and stops when it hits a processed announcement
func (w Worker) processIncremental(ctx context.Context) error {
	ctx, span := w.tracer.Start(ctx, "worker.Worker.processIncremental")
	defer span.End()

	logger := w.logger.With()
	ctx = slogcontext.Append(ctx, slog.String("tag", "worker.Worker.processIncremental"))

	latestDB, err := w.stateStore.LatestAnnouncement(ctx)
	if err != nil {
		return err
	}

	resp, err := w.idxClient.FetchAnnouncements(ctx, 0, latestDB.Date)
	if err != nil {
		return err
	}
	latest := resp.Replies[0].Announcement
	totalItems, pageSize := resp.ResultCount, w.config.App.IDX.PageSize
	totalPages := (totalItems + pageSize) / pageSize
	ctx = slogcontext.Append(ctx,
		slog.Int("total_items", totalItems),
		slog.Int("page_size", pageSize),
		slog.Int("total_pages", totalPages),
		slog.Time("latest_db_date", latestDB.Date),
		slog.Time("latest_announcement", latest.Date),
	)
	span.SetAttributes(
		attribute.Int("total_items", totalItems),
		attribute.Int("page_size", pageSize),
		attribute.Int("total_pages", totalPages),
		attribute.String("latest_db_date", latestDB.Date.String()),
		attribute.String("latest_announcement_date", latest.Date.String()),
	)

	if latest.Date.Before(latestDB.Date) || latest.Date.Equal(latestDB.Date) {
		err := fmt.Errorf(
			"no new announcements to process, latest_db_date=%s, latest_announcement_date=%s",
			latestDB.Date.String(),
			latest.Date.String(),
		)
		logger.InfoContext(ctx, "no new announcements to process", slog.Any("error", err))
		return err
	}

	processedCount := 0
	for page := totalPages; page >= 0; page-- {
		ctx := slogcontext.Append(
			ctx,
			slog.Int("page", page),
			slog.Int("processed_count", processedCount),
		)
		resp, err := w.idxClient.FetchAnnouncements(ctx, page, latestDB.Date)
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

		for _, reply := range resp.Replies {
			ctx := slogcontext.Append(ctx,
				slog.Int("page", page),
				slog.Int("processed_count", processedCount),
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

func (w Worker) processAnnouncement(ctx context.Context, ann models.Announcement) error {
	ctx, span := w.tracer.Start(
		ctx,
		"worker.Worker.processAnnouncement",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("tag", "worker.Worker.processAnnouncement"),
			attribute.String("idx_announcement_id", ann.ID2),
			attribute.String("stock_code", ann.StockCode),
			attribute.String("announcement_date", ann.Date.String()),
		),
	)
	defer span.End()

	ctx = slogcontext.Append(ctx,
		slog.String("tag", "worker.Worker.processAnnouncement"),
		slog.String("idx_announcement_id", ann.ID2),
		slog.String("stock_code", ann.StockCode),
		slog.Time("announcement_date", ann.Date),
	)
	if err := w.stateStore.RecordAnnouncement(ctx, ann); err != nil {
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

func (w Worker) syncMarketDetector(ctx context.Context, symbol string, date time.Time) error {
	ctx, span := w.tracer.Start(ctx, "worker.Worker.syncMarketDetector")
	defer span.End()

	dateStr := date.Format("2006-01-02")

	resp, err := w.stockbitClient.FetchMarketDetector(ctx, symbol, dateStr, dateStr)
	if err != nil {
		return err
	}

	summary := models.MarketDetectorSummary{
		Symbol:        symbol,
		TradeDate:     dateStr,
		AccDistStatus: resp.Data.BandarDetector.BrokerAccDist,
		TotalValue:    resp.Data.BandarDetector.Value,
	}

	var txns []models.BrokerTransaction
	for _, b := range resp.Data.BrokerSummary.BrokersBuy {
		lots := b.BLot.IntPart()
		txns = append(txns, models.BrokerTransaction{
			Symbol:       symbol,
			TradeDate:    dateStr,
			BrokerCode:   b.BrokerCode,
			Side:         "BUY",
			Lots:         lots,
			Frequency:    b.Freq,
			InvestorType: b.InvestorType,
			AvgPrice:     b.BuyAvgPrice,
		})
	}

	for _, b := range resp.Data.BrokerSummary.BrokersSell {
		lots := b.SLot.IntPart()
		txns = append(txns, models.BrokerTransaction{
			Symbol:       symbol,
			TradeDate:    dateStr,
			BrokerCode:   b.BrokerCode,
			Side:         "SELL",
			Lots:         lots,
			Frequency:    b.Freq,
			InvestorType: b.InvestorType,
			AvgPrice:     b.SellAvgPrice,
		})
	}

	if err := w.stateStore.UpsertMarketDetector(ctx, summary, txns); err != nil {
		err = fmt.Errorf("upsert: %w", err)
		return err
	}

	return nil
}

func (w Worker) processAttachment(
	ctx context.Context,
	ann models.Announcement,
	att models.Attachment,
) error {
	cleanExp := regexp.MustCompile(`[^a-zA-Z0-9\.]+`)
	whitespaceExp := regexp.MustCompile(`\s+`)
	datePrefix := ann.Date.Format("2006-01-02")
	stockcode := strings.ToLower(ann.StockCode)
	originalname := strings.ToLower(att.OriginalFilename)
	originalname = cleanExp.ReplaceAllString(originalname, " ")
	originalname = whitespaceExp.ReplaceAllString(originalname, " ")
	originalname = whitespaceExp.ReplaceAllString(originalname, "_")
	originalname = strings.Trim(originalname, "_")
	title := strings.ToLower(ann.AnnouncementTitle)
	title = cleanExp.ReplaceAllString(title, " ")
	title = whitespaceExp.ReplaceAllString(title, " ")
	title = whitespaceExp.ReplaceAllString(title, "_")
	title = strings.Trim(title, "_")
	title = title[:min(64, len(title))]
	title = fmt.Sprintf("%s_%s", datePrefix, title)
	filename := fmt.Sprintf("%s_%s", datePrefix, originalname)
	filePath := filepath.Join(stockcode, title, filename)
	filePath = filepath.Clean(filePath)

	bucket := w.config.MinIO.Bucket
	ctx, span := w.tracer.Start(
		ctx,
		"worker.Worker.processAttachment",
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

	ctx = slogcontext.Append(
		ctx,
		slog.String("idx_filename", att.OriginalFilename),
		slog.String("tag", "worker.Worker.processAttachment"),
		slog.String("idx_filename", att.OriginalFilename),
		slog.String("idx_attachment_url", att.FullSavePath),
		slog.String("bucket", bucket),
		slog.String("filename", filePath),
	)
	logger := w.logger.With()

	data, contentType, err := w.idxClient.DownloadFile(ctx, att.FullSavePath)
	if err != nil {
		err = fmt.Errorf("downloading attachment idx_attachment_url=%s : %w", att.FullSavePath, err)
		return err
	}
	checksum := calculateChecksum(data)
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogcontext.Append(ctx,
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

	if err := w.stateStore.RecordAttachment(ctx, ann.ID2, att, checksum, filePath); err != nil {
		return err
	}

	return nil
}

func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
