package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
		trace.WithAttributes(attribute.String("tag", "worker.Worker.Process")),
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

		logger.DebugContext(ctx, "fetching announcements from page", slog.Int("page", p), slog.Int("index_from", indexFrom))
		span.AddEvent(fmt.Sprintf("fetching announcements from page %d", p))
		pageResp, err := w.idxClient.FetchAnnouncements(ctx, indexFrom)
		if err != nil {
			logger.ErrorContext(ctx, "failed to fetch page during initial seeding", slog.Int("page", p), slog.Int("index_from", indexFrom), slog.Any("error", err))
			continue
		}

		for _, reply := range pageResp.Replies {
			if err := w.processAnnouncement(ctx, reply.Announcement); err != nil {
				continue
			}
			if err := w.stateStore.RecordAnnouncement(ctx, reply.Announcement); err != nil {
				logger.ErrorContext(ctx, "failed to record announcement during initial seeding", slog.String("id2", reply.Announcement.ID2), slog.Any("error", err))
			}
			processedCount++
		}
	}

	logger.InfoContext(ctx, "completed initial seeding", slog.Int("processed_count", processedCount))
	span.AddEvent(fmt.Sprintf("completed initial seeding, processed %d announcements", processedCount))

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

			logger.DebugContext(ctx, "checking if announcement already processed", slog.String("id2", ann.ID2), slog.String("stockCode", ann.StockCode))
			span.AddEvent(fmt.Sprintf("checking if announcement %s already processed", ann.ID2))

			processed, err := w.stateStore.IsProcessed(ctx, ann.ID2)
			if err != nil {
				logger.ErrorContext(ctx, "failed to check if announcement is processed", slog.String("id2", ann.ID2), slog.Any("error", err))
				continue
			}

			if processed {
				logger.DebugContext(ctx, "reached already processed announcement, stopping incremental poll", slog.String("id2", ann.ID2))
				span.AddEvent(fmt.Sprintf("reached already processed announcement %s, stopping", ann.ID2))
				caughtUp = true
				break
			}

			if err := w.processAnnouncement(ctx, ann); err != nil {
				continue
			}
			if err := w.stateStore.RecordAnnouncement(ctx, ann); err != nil {
				logger.ErrorContext(ctx, "failed to record announcement", slog.String("id2", ann.ID2), slog.Any("error", err))
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

	logger.InfoContext(ctx, "completed incremental sync", slog.Int("processed_count", processedCount))
	span.AddEvent(fmt.Sprintf("completed incremental sync, processed %d new announcements", processedCount))

	return nil
}

func (w *Worker) processAnnouncement(ctx context.Context, ann models.Announcement) error {
	ctx, span := w.tracer.Start(ctx, "worker.processAnnouncement")
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.processAnnouncement"))
	logger.InfoContext(ctx, "processing announcement for stock code", slog.String("id2", ann.ID2), slog.String("stockCode", ann.StockCode), slog.Time("date", ann.AnnouncementDate))
	span.AddEvent(fmt.Sprintf("processing announcement %s for stock %s", ann.ID2, ann.StockCode))

	for _, att := range ann.Attachments {
		if err := w.processAttachment(ctx, ann, att); err != nil {
			logger.ErrorContext(ctx, "failed to process attachment", slog.String("filename", att.OriginalFilename), slog.Any("error", err))
		}
	}

	return nil
}

func (w *Worker) syncMarketDetector(ctx context.Context, symbol string, date time.Time) error {
	ctx, span := w.tracer.Start(ctx, "worker.syncMarketDetector")
	defer span.End()

	dateStr := date.Format("2006-01-02")

	logger := w.logger.With(slog.String("tag", "worker.syncMarketDetector"))
	logger.DebugContext(ctx, "fetching market detector data", slog.String("symbol", symbol), slog.String("date", dateStr))
	span.AddEvent(fmt.Sprintf("fetching market detector data for symbol %s on %s", symbol, dateStr))

	resp, err := w.stockbitClient.FetchMarketDetector(ctx, symbol, dateStr, dateStr)
	if err != nil {
		err = fmt.Errorf("fetch: %w", err)
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

	logger.DebugContext(ctx, "upserting market detector data to database", slog.Int("txns", len(txns)))
	span.AddEvent(fmt.Sprintf("upserting market detector data with %d transactions", len(txns)))

	if err := w.stateStore.UpsertMarketDetector(ctx, summary, txns); err != nil {
		err = fmt.Errorf("upsert: %w", err)
		return err
	}

	logger.InfoContext(ctx, "successfully synced market detector data", slog.String("symbol", symbol), slog.String("date", dateStr), slog.Int("txns", len(txns)))
	span.AddEvent(fmt.Sprintf("successfully synced market detector data for symbol %s", symbol))

	return nil
}

func (w Worker) processAttachment(ctx context.Context, ann models.Announcement, att models.Attachment) error {
	ctx, span := w.tracer.Start(ctx, "worker.processAttachment")
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.processAttachment"))
	datePrefix := ann.AnnouncementDate.Format("2006-01-02")
	originalName := strings.ToLower(att.OriginalFilename)
	targetName := fmt.Sprintf("%s_%s_%s", datePrefix, ann.ID2, originalName)

	bucket := w.config.MinIO.Bucket

	logger.DebugContext(ctx, "checking if attachment exists in MinIO storage", slog.String("bucket", bucket), slog.String("filename", targetName))
	span.AddEvent(fmt.Sprintf("checking if attachment %s exists in storage", targetName))

	exists, err := w.storage.Exists(ctx, bucket, targetName)
	if err != nil {
		err = fmt.Errorf("check existence for %s: %w", targetName, err)
		return err
	}

	logger.DebugContext(ctx, "creating bucket if not exists", slog.String("bucket", bucket))
	span.AddEvent(fmt.Sprintf("ensuring bucket %s exists", bucket))

	if err := w.storage.CreateBucket(ctx, bucket); err != nil {
		if !errors.Is(err, storage.ErrBucketExists) {
			logger.ErrorContext(ctx, "failed to create bucket", slog.String("bucket", bucket), slog.Any("error", err))
		}
	}

	if exists {
		logger.DebugContext(ctx, "attachment already exists in storage, skipping download", slog.String("name", targetName))
		span.AddEvent(fmt.Sprintf("attachment %s already exists, skipping", targetName))
		return nil
	}

	logger.DebugContext(ctx, "downloading attachment from URL", slog.String("url", att.FullSavePath))
	span.AddEvent(fmt.Sprintf("downloading attachment from %s", att.FullSavePath))

	data, contentType, err := w.downloadFile(ctx, att.FullSavePath)
	if err != nil {
		err = fmt.Errorf("download from %s: %w", att.FullSavePath, err)
		return err
	}

	logger.DebugContext(ctx, "successfully downloaded attachment", slog.Int("size", len(data)), slog.String("contentType", contentType))
	span.AddEvent(fmt.Sprintf("successfully downloaded attachment, size=%d", len(data)))

	checksum := calculateChecksum(data)

	logger.DebugContext(ctx, "uploading attachment to MinIO", slog.String("bucket", bucket), slog.String("filename", targetName), slog.Int("size", len(data)))
	span.AddEvent(fmt.Sprintf("uploading attachment %s to bucket %s", targetName, bucket))

	if err := w.storage.Upload(ctx, bucket, targetName, bytes.NewReader(data), int64(len(data)), contentType); err != nil {
		err = fmt.Errorf("upload for %s: %w", targetName, err)
		return err
	}

	logger.InfoContext(ctx, "successfully archived attachment", slog.String("name", targetName), slog.Int("size", len(data)))
	span.AddEvent(fmt.Sprintf("successfully archived attachment %s", targetName))

	logger.DebugContext(ctx, "recording attachment metadata in database", slog.String("filename", targetName), slog.String("checksum", checksum))
	span.AddEvent(fmt.Sprintf("recording attachment metadata for %s", targetName))

	if err := w.stateStore.RecordAttachment(ctx, ann.ID2, att, checksum, targetName); err != nil {
		err = fmt.Errorf("record attachment for %s: %w", targetName, err)
		return err
	}

	return nil
}

func (w Worker) downloadFile(ctx context.Context, url string) ([]byte, string, error) {
	ctx, span := w.tracer.Start(ctx, "worker.downloadFile")
	defer span.End()

	logger := w.logger.With(slog.String("tag", "worker.downloadFile"))

	logger.DebugContext(ctx, "downloading file from URL", slog.String("url", url))
	span.AddEvent(fmt.Sprintf("downloading file from %s", url))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download, status: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	data := buf.Bytes()

	logger.DebugContext(ctx, "successfully downloaded file", slog.Int("size", len(data)), slog.String("contentType", contentType))
	span.AddEvent(fmt.Sprintf("successfully downloaded file, size=%d", len(data)))

	return data, contentType, nil
}

func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
