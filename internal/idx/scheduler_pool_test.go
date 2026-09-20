package idx

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// --- Mock AnnouncementStore ---

type mockAnnouncementStore struct {
	latestFn    func(ctx context.Context, db qrm.DB) (model.Announcements, error)
	insertFn    func(ctx context.Context, db qrm.DB, ann ...model.Announcements) error
	processedFn func(ctx context.Context, db qrm.DB, idxIDs ...string) (map[string]bool, error)
	annFn       func(ctx context.Context, db qrm.DB, id ...string) ([]model.Announcements, error)
	existsFn    func(ctx context.Context, db qrm.DB) (bool, error)
}

func (m *mockAnnouncementStore) LatestAnnouncement(ctx context.Context, db qrm.DB) (model.Announcements, error) {
	if m.latestFn != nil {
		return m.latestFn(ctx, db)
	}
	return model.Announcements{}, nil
}

func (m *mockAnnouncementStore) InsertAnnouncement(ctx context.Context, db qrm.DB, ann ...model.Announcements) error {
	if m.insertFn != nil {
		return m.insertFn(ctx, db, ann...)
	}
	return nil
}

func (m *mockAnnouncementStore) IsProcessed(ctx context.Context, db qrm.DB, idxIDs ...string) (map[string]bool, error) {
	if m.processedFn != nil {
		return m.processedFn(ctx, db, idxIDs...)
	}
	return map[string]bool{}, nil
}

func (m *mockAnnouncementStore) Announcement(ctx context.Context, db qrm.DB, id ...string) ([]model.Announcements, error) {
	if m.annFn != nil {
		return m.annFn(ctx, db, id...)
	}
	return nil, nil
}

func (m *mockAnnouncementStore) IsExists(ctx context.Context, db qrm.DB) (bool, error) {
	if m.existsFn != nil {
		return m.existsFn(ctx, db)
	}
	return false, nil
}

// --- Mock AttachmentStore ---

type mockAttachmentStore struct {
	unprocessedFn func(ctx context.Context) ([]model.Attachments, error)
	claimFn       func(ctx context.Context, id ...uuid.UUID) ([]model.Attachments, error)
	insertFn      func(ctx context.Context, db qrm.DB, attachments ...model.Attachments) error
	updateFn      func(ctx context.Context, attachment *model.Attachments) error
	attFn         func(ctx context.Context, db qrm.DB, id ...uuid.UUID) (model.Attachments, error)
}

func (m *mockAttachmentStore) UnprocessedAttachments(ctx context.Context) ([]model.Attachments, error) {
	if m.unprocessedFn != nil {
		return m.unprocessedFn(ctx)
	}
	return nil, nil
}

func (m *mockAttachmentStore) ClaimAttachments(ctx context.Context, id ...uuid.UUID) ([]model.Attachments, error) {
	if m.claimFn != nil {
		return m.claimFn(ctx, id...)
	}
	return nil, nil
}

func (m *mockAttachmentStore) InsertAttachment(ctx context.Context, db qrm.DB, attachments ...model.Attachments) error {
	if m.insertFn != nil {
		return m.insertFn(ctx, db, attachments...)
	}
	return nil
}

func (m *mockAttachmentStore) UpdateAttachmentResult(ctx context.Context, attachment *model.Attachments) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, attachment)
	}
	return nil
}

func (m *mockAttachmentStore) Attachment(ctx context.Context, db qrm.DB, id ...uuid.UUID) (model.Attachments, error) {
	if m.attFn != nil {
		return m.attFn(ctx, db, id...)
	}
	return model.Attachments{}, nil
}

// --- Mock Client ---

type mockClient struct {
	fetchFn   func(ctx context.Context, page int, dateFrom time.Time) (AnnouncementResponse, error)
	downloadFn func(ctx context.Context, url string) ([]byte, string, error)
}

func (m *mockClient) FetchAnnouncements(ctx context.Context, page int, dateFrom time.Time) (AnnouncementResponse, error) {
	if m.fetchFn != nil {
		return m.fetchFn(ctx, page, dateFrom)
	}
	return AnnouncementResponse{}, nil
}

func (m *mockClient) DownloadFile(ctx context.Context, url string) ([]byte, string, error) {
	if m.downloadFn != nil {
		return m.downloadFn(ctx, url)
	}
	return nil, "", nil
}

// --- Mock AttachmentWorker ---

type mockAttachmentWorker struct {
	workFn func(ctx context.Context, task *AttachmentTask) (AttachmentResult, error)
	count  int
	mu     sync.Mutex
}

func (m *mockAttachmentWorker) Work(ctx context.Context, task *AttachmentTask) (AttachmentResult, error) {
	m.mu.Lock()
	m.count++
	m.mu.Unlock()

	if m.workFn != nil {
		return m.workFn(ctx, task)
	}
	return AttachmentResult{AttachmentTask: task}, nil
}

func (m *mockAttachmentWorker) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

// --- Test helpers ---

func newNoopLogger() *slog.Logger {
	return slog.Default()
}

func newNoopTracer() *noopTracer {
	return &noopTracer{}
}

type noopTracer = tracenoop.Tracer

func newTestMetricsProvider(t interface{ Helper() }) *telemetry.MetricsProvider {
	t.Helper()
	meter := noop.NewMeterProvider().Meter("test")
	metrics, err := telemetry.NewMetrics(meter)
	if err != nil {
		panic("failed to create test metrics: " + err.Error())
	}
	return metrics
}

func newTestAnnouncement(idxID string) Announcement {
	return Announcement{
		ID:                idxID,
		AnnouncementTitle: "Test Announcement " + idxID,
		AnnouncementType:  "Type A",
		StockCode:         "BBCA",
		IsStock:           true,
		Date:              time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		CreatedDate:       time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		Attachments: []Attachment{
			{
				PDFFilename:      "test.pdf",
				FullSavePath:     "https://example.com/test.pdf",
				OriginalFilename: "test.pdf",
			},
		},
	}
}

func newTestAttachment(id uuid.UUID) model.Attachments {
	return model.Attachments{
		ID:                id,
		IdxAnnouncementID: "ann-1",
		IdxURL:            "https://example.com/test.pdf",
		OriginalFilename:  "test.pdf",
		Filename:          "test.pdf",
		Title:             "Test Attachment",
		StockCode:         "BBCA",
		Date:              time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
	}
}
