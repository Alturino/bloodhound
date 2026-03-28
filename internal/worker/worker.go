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

// Process executes one cycle of polling and processing with reverse paging
func (w *Worker) Process(ctx context.Context) error {
	w.logger.DebugContext(ctx, "starting processing cycle")

	// 1. Fetch first page to determine ResultCount
	resp, err := w.idxClient.FetchAnnouncements(ctx, 1)
	if err != nil {
		return fmt.Errorf("failed to fetch initial page: %w", err)
	}

	if resp.ResultCount == 0 || len(resp.Replies) == 0 {
		w.logger.DebugContext(ctx, "no announcements found")
		return nil
	}

	totalItems := resp.ResultCount

	// 2. Calculate the total number of pages
	// Example: total=25, size=10 -> totalPages = (25 + 10 - 1) / 10 = 3
	pageSize := w.config.App.IDX.PageSize
	totalPages := (totalItems + pageSize - 1) / pageSize
	
	w.logger.InfoContext(ctx, "processing historical announcements", 
		slog.Int("total_items", totalItems),
		slog.Int("page_size", pageSize),
		slog.Int("total_pages", totalPages),
	)

	// 3. Iterate from the last page down to page 1
	for currPage := totalPages; currPage >= 1; currPage-- {
		w.logger.DebugContext(ctx, "fetching page", slog.Int("page", currPage))
		
		pageResp, err := w.idxClient.FetchAnnouncements(ctx, currPage)
		if err != nil {
			w.logger.ErrorContext(ctx, "failed to fetch page", slog.Int("page", currPage), slog.Any("error", err))
			continue
		}

		// Process announcements on this page (IDX returns newest first on EACH page usually).
		// Per user request, we don't have to reverse this inner loop.
		for i := 0; i < len(pageResp.Replies); i++ {
			ann := pageResp.Replies[i].Announcement
			
			// Check if already processed
			processed, err := w.stateStore.IsProcessed(ctx, ann.ID2)
			if err != nil {
				w.logger.ErrorContext(ctx, "failed to check if processed", slog.String("id2", ann.ID2), slog.Any("error", err))
				continue
			}
			
			if processed {
				continue
			}

			if err := w.processAnnouncement(ctx, ann); err != nil {
				w.logger.ErrorContext(ctx, "failed to process announcement",
					slog.String("id2", ann.ID2),
					slog.Any("error", err),
				)
				continue
			}

			// Record as processed
			if err := w.stateStore.RecordAnnouncement(ctx, ann); err != nil {
				w.logger.ErrorContext(ctx, "failed to record announcement",
					slog.String("id2", ann.ID2),
					slog.Any("error", err),
				)
			}
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

	if !exists {
		// Upload
		err = w.storage.Upload(ctx, bucket, targetName, bytes.NewReader(data), int64(len(data)), contentType)
		if err != nil {
			return fmt.Errorf("failed to upload file: %w", err)
		}
		w.logger.InfoContext(ctx, "successfully archived attachment", slog.String("name", targetName))
	} else {
		w.logger.DebugContext(ctx, "file already exists in storage, skipping upload", slog.String("name", targetName))
	}

	// Record in database
	if err := w.stateStore.RecordAttachment(ctx, ann.ID2, att, checksum, targetName); err != nil {
		return fmt.Errorf("failed to record attachment status: %w", err)
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
