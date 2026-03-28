package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
)

// IDXClient defines the subset of IDX client methods needed by the worker
type IDXClient interface {
	FetchAnnouncements(ctx context.Context, indexFrom int) (models.AnnouncementResponse, error)
}

// StockbitClient defines the subset of Stockbit client methods needed by the worker
type StockbitClient interface {
	FetchMarketDetector(ctx context.Context, symbol, dateFrom, dateTo string) (models.StockbitMarketDetectorResponse, error)
}

// Worker handles the orchestration of fetching and processing announcements
type Worker struct {
	idxClient      IDXClient
	stockbitClient StockbitClient
	storage        storage.Storage
	stateStore     state.Store
	config         *config.Config
	logger         *slog.Logger
}

// NewWorker initializes a new background worker
func NewWorker(
	idxClient IDXClient,
	stockbitClient StockbitClient,
	storage storage.Storage,
	stateStore state.Store,
	cfg *config.Config,
	logger *slog.Logger,
) *Worker {
	return &Worker{
		idxClient:      idxClient,
		stockbitClient: stockbitClient,
		storage:        storage,
		stateStore:     stateStore,
		config:         cfg,
		logger:         logger,
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
	w.logger.DebugContext(ctx, "starting processing cycle")

	hasData, err := w.stateStore.HasSavedAnnouncements(ctx)
	if err != nil {
		return fmt.Errorf("check if store has data: %w", err)
	}

	if !hasData {
		return w.processInitial(ctx)
	}

	return w.processIncremental(ctx)
}

// processInitial performs a full scan from last page to first to seed the database
func (w *Worker) processInitial(ctx context.Context) error {
	w.logger.InfoContext(ctx, "starting initial deep scan for seeding")

	resp, err := w.idxClient.FetchAnnouncements(ctx, 1)
	if err != nil {
		return fmt.Errorf("initial fetch: %w", err)
	}

	totalItems := resp.ResultCount
	pageSize := w.config.App.IDX.PageSize
	totalPages := (totalItems + pageSize - 1) / pageSize

	w.logger.InfoContext(ctx, "seeding historical announcements",
		slog.Int("total_items", totalItems),
		slog.Int("page_size", pageSize),
		slog.Int("total_pages", totalPages),
	)

	for p := totalPages; p >= 1; p-- {
		indexFrom := (p-1)*pageSize + 1
		pageResp, err := w.idxClient.FetchAnnouncements(ctx, indexFrom)
		if err != nil {
			w.logger.ErrorContext(ctx, "fetch page during seed", slog.Int("page", p), slog.Int("index_from", indexFrom), slog.Any("error", err))
			continue
		}

		for _, reply := range pageResp.Replies {
			if err := w.processAnnouncement(ctx, reply.Announcement); err != nil {
				continue
			}
			if err := w.stateStore.RecordAnnouncement(ctx, reply.Announcement); err != nil {
				w.logger.ErrorContext(ctx, "record during seed", slog.String("id2", reply.Announcement.ID2), slog.Any("error", err))
			}
		}
	}

	return nil
}

// processIncremental polls page 1 and stops when it hits a processed announcement
func (w *Worker) processIncremental(ctx context.Context) error {
	w.logger.DebugContext(ctx, "starting incremental poll")

	currPage := 1
	for {
		resp, err := w.idxClient.FetchAnnouncements(ctx, currPage)
		if err != nil {
			return fmt.Errorf("incremental fetch at page %d: %w", currPage, err)
		}

		if len(resp.Replies) == 0 {
			break
		}

		caughtUp := false
		for _, reply := range resp.Replies {
			ann := reply.Announcement

			processed, err := w.stateStore.IsProcessed(ctx, ann.ID2)
			if err != nil {
				w.logger.ErrorContext(ctx, "processed check", slog.String("id2", ann.ID2), slog.Any("error", err))
				continue
			}

			if processed {
				w.logger.DebugContext(ctx, "reached already processed announcement, stopping", slog.String("id2", ann.ID2))
				caughtUp = true
				break
			}

			if err := w.processAnnouncement(ctx, ann); err != nil {
				continue
			}
			if err := w.stateStore.RecordAnnouncement(ctx, ann); err != nil {
				w.logger.ErrorContext(ctx, "record announcement", slog.String("id2", ann.ID2), slog.Any("error", err))
			}
		}

		if caughtUp {
			break
		}

		currPage++
		// Safety break to avoid infinite loops if something goes wrong
		if currPage > 10 {
			break
		}
	}

	return nil
}

func (w *Worker) processAnnouncement(ctx context.Context, ann models.Announcement) error {
	w.logger.InfoContext(ctx, "processing announcement",
		slog.String("id2", ann.ID2),
		slog.String("stock_code", ann.StockCode),
		slog.Time("date", ann.AnnouncementDate),
	)

	for _, att := range ann.Attachments {
		if err := w.processAttachment(ctx, ann, att); err != nil {
			w.logger.ErrorContext(ctx, "process attachment",
				slog.String("filename", att.OriginalFilename),
				slog.Any("error", err),
			)
		}
	}

	if err := w.syncMarketDetector(ctx, ann.StockCode, ann.AnnouncementDate); err != nil {
		w.logger.ErrorContext(ctx, "failed to sync market detector",
			slog.String("symbol", ann.StockCode),
			slog.Any("error", err),
		)
	}

	return nil
}

func (w *Worker) syncMarketDetector(ctx context.Context, symbol string, date time.Time) error {
	dateStr := date.Format("2006-01-02")
	
	resp, err := w.stockbitClient.FetchMarketDetector(ctx, symbol, dateStr, dateStr)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
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
		return fmt.Errorf("upsert: %w", err)
	}

	w.logger.InfoContext(ctx, "synced market detector data",
		slog.String("symbol", symbol),
		slog.String("date", dateStr),
		slog.Int("txns", len(txns)),
	)

	return nil
}

func (w *Worker) processAttachment(ctx context.Context, ann models.Announcement, att models.Attachment) error {
	// Rename file: yyyy-MM-dd_{id2}_original_filename.ext
	datePrefix := ann.AnnouncementDate.Format("2006-01-02")
	originalName := strings.ToLower(att.OriginalFilename)
	targetName := fmt.Sprintf("%s_%s_%s", datePrefix, ann.ID2, originalName)

	// Check if exists in storage BEFORE downloading
	bucket := w.config.MinIO.Bucket
	exists, err := w.storage.Exists(ctx, bucket, targetName)
	if err != nil {
		return fmt.Errorf("check existence for %s: %w", targetName, err)
	}

	if exists {
		w.logger.DebugContext(ctx, "file already exists and is not empty, skipping download", slog.String("name", targetName))
		return nil
	}

	// Download file
	data, contentType, err := w.downloadFile(ctx, att.FullSavePath)
	if err != nil {
		return fmt.Errorf("download from %s: %w", att.FullSavePath, err)
	}

	// Calculate checksum for DB record
	checksum := calculateChecksum(data)

	// Upload
	err = w.storage.Upload(ctx, bucket, targetName, bytes.NewReader(data), int64(len(data)), contentType)
	if err != nil {
		return fmt.Errorf("upload for %s: %w", targetName, err)
	}

	w.logger.InfoContext(ctx, "successfully archived attachment", slog.String("name", targetName))

	// Record in database
	if err := w.stateStore.RecordAttachment(ctx, ann.ID2, att, checksum, targetName); err != nil {
		return fmt.Errorf("record attachment for %s: %w", targetName, err)
	}

	return nil
}

func (w *Worker) downloadFile(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	// Some IDX files might need basic headers or impersonation if they block simple clients
	// For now, let's try a standard Get
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download, status: %d", resp.StatusCode)
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
)

// IDXClient defines the subset of IDX client methods needed by the worker
type IDXClient interface {
	FetchAnnouncements(ctx context.Context, indexFrom int) (models.AnnouncementResponse, error)
}

// Worker handles the orchestration of fetching and processing announcements
type Worker struct {
	idxClient  IDXClient
	storage    storage.Storage
	stateStore state.Store
	config     *config.Config
	logger     *slog.Logger
}

// NewWorker creates a new background worker
func NewWorker(idxClient IDXClient, storage storage.Storage, stateStore state.Store, cfg *config.Config, logger *slog.Logger) *Worker {
	return &Worker{
		idxClient:  idxClient,
		storage:    storage,
		stateStore: stateStore,
		config:     cfg,
		logger:     logger,
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
		w.logger.ErrorContext(ctx, "failed initial processing", slog.Any("error", err))
	}

	for {
		select {
		case <-ctx.Done():
			w.logger.InfoContext(ctx, "stopping background worker")
			return ctx.Err()
		case <-ticker.C:
			if err := w.Process(ctx); err != nil {
				w.logger.ErrorContext(ctx, "failed processing", slog.Any("error", err))
			}
		}
	}
}

// Process executes one cycle of polling and processing
func (w *Worker) Process(ctx context.Context) error {
	lastID, err := w.stateStore.GetLastProcessedID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get last processed ID: %w", err)
	}

	w.logger.DebugContext(ctx, "starting processing cycle", slog.String("last_id", lastID))

	// Fetch first page
	resp, err := w.idxClient.FetchAnnouncements(ctx, 1)
	if err != nil {
		return fmt.Errorf("failed to fetch announcements: %w", err)
	}

	if len(resp.Replies) == 0 {
		w.logger.DebugContext(ctx, "no announcements found")
		return nil
	}

	newAnnouncements := make([]models.Reply, 0)
	for _, reply := range resp.Replies {
		if reply.Announcement.ID2 == lastID {
			break
		}
		newAnnouncements = append(newAnnouncements, reply)
	}

	if len(newAnnouncements) == 0 {
		w.logger.DebugContext(ctx, "no new announcements since last run")
		return nil
	}

	w.logger.InfoContext(ctx, "found new announcements", slog.Int("count", len(newAnnouncements)))

	// Process from oldest to newest to maintain state correctness if interrupted
	for i := len(newAnnouncements) - 1; i >= 0; i-- {
		reply := newAnnouncements[i]
		if err := w.processAnnouncement(ctx, reply.Announcement); err != nil {
			w.logger.ErrorContext(ctx, "failed to process announcement",
				slog.String("id2", reply.Announcement.ID2),
				slog.Any("error", err),
			)
			continue
		}

		// Update state after each successful announcement processing
		if err := w.stateStore.SetLastProcessedID(ctx, reply.Announcement.ID2); err != nil {
			w.logger.ErrorContext(ctx, "failed to update state",
				slog.String("id2", reply.Announcement.ID2),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

func (w *Worker) processAnnouncement(ctx context.Context, ann models.Announcement) error {
	w.logger.InfoContext(ctx, "processing announcement",
		slog.String("id2", ann.ID2),
		slog.String("stock_code", ann.StockCode),
		slog.Time("date", ann.AnnouncementDate),
	)

	for _, att := range ann.Attachments {
		if err := w.processAttachment(ctx, ann, att); err != nil {
			w.logger.ErrorContext(ctx, "failed to process attachment",
				slog.String("filename", att.OriginalFilename),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

func (w *Worker) processAttachment(ctx context.Context, ann models.Announcement, att models.Attachment) error {
	// Download file
	data, contentType, err := w.downloadFile(ctx, att.FullSavePath)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	// Calculate checksum
	checksum := calculateChecksum(data)
	shortChecksum := checksum[:8]

	// Rename file: yyyy-MM-dd_KODE_EMITEN_checksum_original_filename.ext
	datePrefix := ann.AnnouncementDate.Format("2006-01-02")
	stockCode := strings.TrimSpace(ann.StockCode)
	originalName := strings.ToLower(att.OriginalFilename)
	
	targetName := fmt.Sprintf("%s_%s_%s_%s", datePrefix, stockCode, shortChecksum, originalName)

	// Check if exists in storage
	bucket := w.config.MinIO.Bucket
	exists, err := w.storage.Exists(ctx, bucket, targetName)
	if err != nil {
		return fmt.Errorf("failed to check existence: %w", err)
	}

	if exists {
		w.logger.DebugContext(ctx, "file already exists in storage, skipping", slog.String("name", targetName))
		return nil
	}

	// Upload
	err = w.storage.Upload(ctx, w.bucket, targetName, bytes.NewReader(data), int64(len(data)), contentType)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}

func (w *Worker) downloadFile(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	// Some IDX files might need basic headers or impersonation if they block simple clients
	// For now, let's try a standard Get
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to download, status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return data, contentType, nil
}

func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
