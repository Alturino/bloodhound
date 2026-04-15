package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
func (w *Worker) Start(ctx context.Context) error {
	interval := w.config.Scheduler.Interval
	w.logger.InfoContext(ctx, "starting background worker", slog.Duration("interval", interval))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run once immediately
	if err := w.Process(ctx); err != nil {
		w.logger.ErrorContext(ctx, "initial processing", slog.Any("error", err))
		return err
	}

	for {
		select {
		case <-ctx.Done():
			w.logger.InfoContext(ctx, "stopping background worker")
			return ctx.Err()
		case <-ticker.C:
			if err := w.Process(ctx); err != nil {
				w.logger.ErrorContext(ctx, "processing cycle", slog.Any("error", err))
			}
		}
	}
}

// Process executes one cycle of polling and processing
func (w *Worker) Process(ctx context.Context) error {
	ctx, span := w.tracer.Start(
		ctx,
		"worker.Worker.Process",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.Worker.Process"))
	logger.DebugContext(ctx, "starting processing cycle to fetch announcements from IDX")
	span.AddEvent("starting processing cycle to fetch announcements from IDX")

	logger.DebugContext(ctx, "checking database for existing announcements to determine sync mode")
	span.AddEvent("checking database for existing announcements to determine sync mode")
	shouldUpdate, err := w.stateStore.ShouldUpdate(ctx)
	if err != nil {
		err = fmt.Errorf("check if store has data: %w", err)
		return err
	}

	if !shouldUpdate {
		return w.processInitial(ctx)
	}

	return w.processIncremental(ctx)
}

// processInitial performs a full scan from last page to first to seed the database
func (w *Worker) processInitial(ctx context.Context) error {
	ctx, span := w.tracer.Start(ctx, "worker.processInitial")
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.processInitial"))
	logger.InfoContext(ctx, "starting initial deep scan for seeding announcements from IDX")
	span.AddEvent("starting initial deep scan for seeding announcements from IDX")

	resp, err := w.idxClient.FetchAnnouncements(ctx, 1)
	if err != nil {
		err = fmt.Errorf("initial fetch: %w", err)
		return err
	}

	totalItems, pageSize := resp.ResultCount, w.config.App.IDX.PageSize
	totalPages := (totalItems + pageSize - 1) / pageSize
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

	logger.DebugContext(ctx, "seeding historical announcements")
	span.AddEvent("seeding historical announcements")
	processedCount := 0
	for p := totalPages; p >= 1; p-- {
		indexFrom := (p-1)*pageSize + 1

		logger.DebugContext(
			ctx,
			"fetching announcements from page",
			slog.Int("page", p),
			slog.Int("index_from", indexFrom),
		)
		span.AddEvent(fmt.Sprintf("fetching announcements from page %d", p))
		pageResp, err := w.idxClient.FetchAnnouncements(ctx, indexFrom)
		if err != nil {
			logger.ErrorContext(
				ctx,
				"failed to fetch page during initial seeding",
				slog.Int("page", p),
				slog.Int("index_from", indexFrom),
				slog.Any("error", err),
			)
			continue
		}

		for _, reply := range pageResp.Replies {
			if err := w.processAnnouncement(ctx, reply.Announcement); err != nil {
				continue
			}
			if err := w.stateStore.RecordAnnouncement(ctx, reply.Announcement); err != nil {
				logger.ErrorContext(
					ctx,
					"failed to record announcement during initial seeding",
					slog.String("idx_announcement_id", reply.Announcement.ID2),
					slog.Any("error", err),
				)
			}
			processedCount++
		}
	}

	logger.InfoContext(
		ctx,
		"completed initial seeding",
		slog.Int("processed_count", processedCount),
	)
	span.AddEvent(
		fmt.Sprintf("completed initial seeding, processed %d announcements", processedCount),
	)

	return nil
}

// processIncremental polls page 1 and stops when it hits a processed announcement
func (w *Worker) processIncremental(ctx context.Context) error {
	ctx, span := w.tracer.Start(ctx, "worker.processIncremental")
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.processIncremental"))
	logger.InfoContext(ctx, "starting incremental poll for new announcements")
	span.AddEvent("starting incremental poll for new announcements")

	currPage := 1
	processedCount := 0
	for {
		logger.DebugContext(ctx, "fetching announcements from page", slog.Int("page", currPage))
		span.AddEvent(fmt.Sprintf("fetching announcements from page %d", currPage))

		resp, err := w.idxClient.FetchAnnouncements(ctx, currPage)
		if err != nil {
			err = fmt.Errorf("incremental fetch at page %d: %w", currPage, err)
			return err
		}

		if len(resp.Replies) == 0 {
			break
		}

		caughtUp := false
		for _, reply := range resp.Replies {
			ann := reply.Announcement

			logger := logger.With(
				slog.String("idx_announcement_id", ann.ID2),
				slog.String("stock_code", ann.StockCode),
			)

			logger.DebugContext(ctx, "is announcement processed")
			span.AddEvent(
				"is announcement processed",
				trace.WithAttributes(attribute.String("idx_announcement_id", ann.ID2)),
			)
			processed, err := w.stateStore.IsProcessed(ctx, ann.ID2)
			if err != nil {
				err = fmt.Errorf("announcement is processed: %w", err)
				logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
				continue
			}
			if processed {
				logger.DebugContext(ctx, "reached already processed announcement, stopping")
				span.AddEvent(
					"reached already processed announcement, stopping",
					trace.WithAttributes(attribute.String("idx_announcement_id", ann.ID2)),
				)
				caughtUp = true
				break
			}

			if err := w.processAnnouncement(ctx, ann); err != nil {
				continue
			}
			if err := w.stateStore.RecordAnnouncement(ctx, ann); err != nil {
				logger.ErrorContext(
					ctx,
					"failed to record announcement",
					slog.String("idx_announcement_id", ann.ID2),
					slog.Any("error", err),
				)
			}
			processedCount++
		}

		if caughtUp {
			break
		}

		currPage++
		if currPage > 10 {
			break
		}
	}

	logger.InfoContext(
		ctx,
		"completed incremental sync",
		slog.Int("processed_count", processedCount),
	)
	span.AddEvent(
		fmt.Sprintf("completed incremental sync, processed %d new announcements", processedCount),
	)

	return nil
}

func (w *Worker) processAnnouncement(ctx context.Context, ann models.Announcement) error {
	ctx, span := w.tracer.Start(
		ctx,
		"worker.processAnnouncement",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("tag", "worker.processAnnouncement"),
			attribute.String("idx_announcement_id", ann.ID2),
			attribute.String("stock_code", ann.StockCode),
			attribute.String("announcement_date", ann.AnnouncementDate.String()),
		),
	)
	defer span.End()

	logger := w.logger.With(
		slog.String("tag", "worker.processAnnouncement"),
		slog.String("idx_announcement_id", ann.ID2),
		slog.String("stock_code", ann.StockCode),
		slog.Time("announcement_date", ann.AnnouncementDate),
	)
	logger.InfoContext(ctx, "processing announcement")
	span.AddEvent(
		"processing announcement for stock code",
		trace.WithAttributes(
			attribute.String("idx_announcement_id", ann.ID2),
			attribute.String("stock_code", ann.StockCode),
		),
	)

	logger.DebugContext(ctx, "processing attachments in announcement")
	var err error
	for _, att := range ann.Attachments {
		if processErr := w.processAttachment(ctx, ann, att); processErr != nil {
			processErr = fmt.Errorf("process attachment %s: %w", att.OriginalFilename, processErr)
			err = errors.Join(err, processErr)
		}
	}
	logger.InfoContext(ctx, "processed attachments in announcement")

	return err
}

func (w *Worker) syncMarketDetector(ctx context.Context, symbol string, date time.Time) error {
	ctx, span := w.tracer.Start(ctx, "worker.syncMarketDetector")
	defer span.End()

	dateStr := date.Format("2006-01-02")

	logger := w.logger.With(slog.String("tag", "worker.syncMarketDetector"))
	logger.DebugContext(
		ctx,
		"fetching market detector data",
		slog.String("symbol", symbol),
		slog.String("date", dateStr),
	)
	span.AddEvent(fmt.Sprintf("fetching market detector data for symbol %s on %s", symbol, dateStr))

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

	logger.DebugContext(
		ctx,
		"upserting market detector data to database",
		slog.Int("txns", len(txns)),
	)
	span.AddEvent(fmt.Sprintf("upserting market detector data with %d transactions", len(txns)))

	if err := w.stateStore.UpsertMarketDetector(ctx, summary, txns); err != nil {
		err = fmt.Errorf("upsert: %w", err)
		return err
	}

	logger.InfoContext(
		ctx,
		"successfully synced market detector data",
		slog.String("symbol", symbol),
		slog.String("date", dateStr),
		slog.Int("txns", len(txns)),
	)
	span.AddEvent(fmt.Sprintf("successfully synced market detector data for symbol %s", symbol))

	return nil
}

func (w Worker) processAttachment(
	ctx context.Context,
	ann models.Announcement,
	att models.Attachment,
) error {
	bucket := w.config.MinIO.Bucket
	datePrefix := ann.AnnouncementDate.Format("2006-01-02")
	originalName := strings.ToLower(att.OriginalFilename)
	filename := fmt.Sprintf(
		"%s/%s_%s_%s",
		strings.ToLower(ann.StockCode),
		datePrefix,
		strings.ToLower(ann.AnnouncementTitle),
		originalName,
	)

	ctx, span := w.tracer.Start(
		ctx,
		"worker.processAttachment",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("idx_filename", att.OriginalFilename),
			attribute.String("idx_attachment_url", att.FullSavePath),
			attribute.String("announcement_title", ann.AnnouncementTitle),
			attribute.String("bucket", bucket),
			attribute.String("filename", filename),
		),
	)
	defer span.End()

	logger := w.logger.With(
		slog.String("tag", "worker.processAttachment"),
		slog.String("idx_filename", att.OriginalFilename),
		slog.String("idx_attachment_url", att.FullSavePath),
		slog.String("bucket", bucket),
		slog.String("filename", filename),
	)

	logger.DebugContext(ctx, "creating bucket")
	span.AddEvent("creating bucket")
	if err := w.storage.CreateBucket(ctx, bucket); err != nil {
		err = fmt.Errorf("creating bucket %s: %w", bucket, err)
		return err
	}
	logger.DebugContext(ctx, "created bucket")
	span.AddEvent("created bucket")

	logger.DebugContext(ctx, "downloading attachment")
	span.AddEvent("downloading attachment")
	data, contentType, err := w.idxClient.DownloadFile(ctx, att.FullSavePath)
	if err != nil {
		err = fmt.Errorf("downloading attachment idx_attachment_url=%s : %w", att.FullSavePath, err)
		return err
	}
	checksum := calculateChecksum(data)
	if logger.Enabled(ctx, slog.LevelDebug) {
		logger = logger.With(
			slog.Int("size", len(data)),
			slog.String("contentType", contentType),
			slog.String("checksum_sha256", checksum),
		)
	}
	logger.DebugContext(ctx, "downloaded attachment")
	span.AddEvent("downloaded attachment")

	logger.DebugContext(ctx, "uploading attachment")
	span.AddEvent("uploading attachment")
	if err := w.storage.Upload(
		ctx,
		bucket,
		filename,
		bytes.NewReader(data),
		int64(len(data)),
		contentType,
	); err != nil {
		err = fmt.Errorf("uploading attachment: %w", err)
		return err
	}
	logger.DebugContext(ctx, "uploaded attachment")
	span.AddEvent("uploaded attachment")

	logger.InfoContext(ctx, "archived attachment")
	span.AddEvent("archived attachment")

	logger.DebugContext(ctx, "recording attachment metadata in database")
	span.AddEvent(fmt.Sprintf("recording attachment metadata for %s", filename))
	if err := w.stateStore.RecordAttachment(ctx, ann.ID2, att, checksum, filename); err != nil {
		err = fmt.Errorf("record attachment for %s: %w", filename, err)
		return err
	}
	logger.DebugContext(ctx, "recorded attachment metadata in database")
	span.AddEvent("recorded attachment metadata in database")

	return nil
}

func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
