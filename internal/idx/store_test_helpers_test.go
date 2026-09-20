package idx

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

type testDB struct {
	db      *sql.DB
	cleanup func()
}

func setupTestDB(t *testing.T) testDB {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("bloodhound_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}

	// Create tables
	if err := createTables(ctx, db); err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	cleanup := func() {
		db.Close()
		pgContainer.Terminate(context.Background())
	}
	t.Cleanup(cleanup)

	return testDB{db: db, cleanup: cleanup}
}

func createTables(ctx context.Context, db *sql.DB) error {
	announcements := `
	CREATE TABLE IF NOT EXISTS announcements (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		idx_id TEXT NOT NULL UNIQUE,
		stock_code TEXT NOT NULL,
		title TEXT NOT NULL,
		date TIMESTAMPTZ NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
	);`

	attachments := `
	CREATE TABLE IF NOT EXISTS attachments (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		idx_announcement_id TEXT NOT NULL REFERENCES announcements(idx_id) ON DELETE CASCADE,
		idx_url TEXT NOT NULL,
		error TEXT NOT NULL,
		original_filename TEXT NOT NULL,
		filename TEXT NOT NULL,
		checksum TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		title TEXT NOT NULL,
		stock_code TEXT NOT NULL,
		is_downloaded BOOLEAN NOT NULL,
		is_processing BOOLEAN NOT NULL DEFAULT false,
		date TIMESTAMPTZ NOT NULL,
		uploaded_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
	);`

	if _, err := db.ExecContext(ctx, announcements); err != nil {
		return fmt.Errorf("create announcements table: %w", err)
	}
	if _, err := db.ExecContext(ctx, attachments); err != nil {
		return fmt.Errorf("create attachments table: %w", err)
	}
	return nil
}

func newTestAnnouncementStoreWithDB(db *sql.DB) AnnouncementStore {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tracer := tracenoop.NewTracerProvider().Tracer("test")
	return NewAnnouncementStore(db, logger, tracer)
}

func newTestAttachmentStoreWithDB(db *sql.DB) AttachmentStore {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tracer := tracenoop.NewTracerProvider().Tracer("test")
	return NewAttachmentStore(db, logger, tracer)
}

func newTestAnnouncement(overrides ...func(*model.Announcements)) model.Announcements {
	ann := model.Announcements{
		ID:        uuid.New(),
		IdxID:     "idx-ann-001",
		StockCode: "BBCA",
		Title:     "Test Announcement",
		Date:      time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
	}
	for _, override := range overrides {
		override(&ann)
	}
	return ann
}

func newTestAttachment(overrides ...func(*model.Attachments)) model.Attachments {
	att := model.Attachments{
		ID:                uuid.New(),
		IdxAnnouncementID: "idx-ann-001",
		IdxURL:            "https://example.com/file.pdf",
		Error:             "",
		OriginalFilename:  "laporan.pdf",
		Filename:          "laporan.pdf",
		Checksum:          "",
		StoragePath:       "",
		Title:             "Test Attachment",
		StockCode:         "BBCA",
		IsDownloaded:      false,
		IsProcessing:      false,
		Date:              time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		UploadedAt:        time.Time{},
	}
	for _, override := range overrides {
		override(&att)
	}
	return att
}
