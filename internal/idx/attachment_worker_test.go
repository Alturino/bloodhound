package idx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/alturino/bloodhound/internal/blobstorage"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type stubClient struct {
	data        []byte
	contentType string
	err         error
}

func (s *stubClient) FetchAnnouncements(context.Context, int, time.Time) (AnnouncementResponse, error) {
	return AnnouncementResponse{}, nil
}

func (s *stubClient) DownloadFile(context.Context, string) ([]byte, string, error) {
	return s.data, s.contentType, s.err
}

type stubStorage struct {
	result blobstorage.SaveResult
	err    error
}

func (s *stubStorage) SaveReader(context.Context, string, io.Reader, int64, string) (blobstorage.SaveResult, error) {
	return s.result, s.err
}

func (s *stubStorage) Exists(context.Context, string) (bool, error) {
	return true, nil
}

func (s *stubStorage) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("unused")
}

func (s *stubStorage) CreateBucket(context.Context) error {
	return nil
}

func newTestMetrics(t *testing.T) *telemetry.Metrics {
	t.Helper()
	meter := noop.NewMeterProvider().Meter("test")
	metrics, err := telemetry.NewMetrics(meter)
	if err != nil {
		t.Fatalf("new metrics: %v", err)
	}
	return metrics
}

func TestAttachmentWork_SetsStoragePathOnSuccess(t *testing.T) {
	date := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	task := AttachmentTask{
		Ctx: context.Background(),
		Attachment: model.Attachments{
			ID:               uuid.New(),
			OriginalFilename: "laporan.pdf",
			Filename:         "laporan.pdf",
			Title:            "Judul Pengumuman",
			StockCode:        "BBCA",
			Date:             date,
		},
	}

	client := &stubClient{data: []byte("pdf-bytes"), contentType: "application/pdf"}
	storage := &stubStorage{}

	worker := NewAttachmentWorker(
		nil,
		slog.Default(),
		tracenoop.NewTracerProvider().Tracer("test"),
		newTestMetrics(t),
		client,
		storage,
	)

	result, err := worker.Work(context.Background(), task)
	if err != nil {
		t.Fatalf("Work: unexpected error: %v", err)
	}

	expected := filepath.Join(
		"bbca",
		"2025-01-02_judul_pengumuman",
		"2025-01-02_laporan.pdf",
	)
	if got := result.Attachment.StoragePath; got != expected {
		t.Errorf("StoragePath = %q, want %q", got, expected)
	}
	if !result.Attachment.IsDownloaded {
		t.Error("IsDownloaded = false, want true")
	}
	if result.Attachment.IsProcessing {
		t.Error("IsProcessing = true, want false")
	}
	if result.Attachment.Error != "" {
		t.Errorf("Error = %q, want empty", result.Attachment.Error)
	}
}

func TestAttachmentWork_LeavesStoragePathEmptyOnDownloadError(t *testing.T) {
	task := AttachmentTask{
		Ctx: context.Background(),
		Attachment: model.Attachments{
			ID:               uuid.New(),
			OriginalFilename: "laporan.pdf",
			Title:            "Judul",
			StockCode:        "BBCA",
			Date:             time.Now(),
		},
	}

	client := &stubClient{err: errors.New("boom")}
	storage := &stubStorage{}

	worker := NewAttachmentWorker(
		nil,
		slog.Default(),
		tracenoop.NewTracerProvider().Tracer("test"),
		newTestMetrics(t),
		client,
		storage,
	)

	result, err := worker.Work(context.Background(), task)
	if err == nil {
		t.Fatal("Work: expected error, got nil")
	}
	if result.Attachment.StoragePath != "" {
		t.Errorf("StoragePath = %q, want empty on download error", result.Attachment.StoragePath)
	}
}
