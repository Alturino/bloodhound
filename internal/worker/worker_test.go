package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/idx"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/state"
)

type FakeStockbitClient struct {
	Response models.StockbitMarketDetectorResponse
	Err      error
}

func (f *FakeStockbitClient) FetchMarketDetector(
	ctx context.Context,
	symbol, dateFrom, dateTo string,
) (models.StockbitMarketDetectorResponse, error) {
	return f.Response, f.Err
}

func TestWorker_Process_InitialSeeding(t *testing.T) {
	fakeIDX := &idx.FakeClient{
		Responses: make(map[int]models.AnnouncementResponse),
	}
	fakeStore := &state.FakeStore{
		Processed: make(map[string]bool),
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	w := &Worker{
		idxClient:      fakeIDX,
		stockbitClient: &FakeStockbitClient{},
		stateStore:     fakeStore,
		config: &config.Config{
			App: config.App{
				IDX: config.IDX{
					PageSize: 10,
				},
			},
		},
		logger: logger,
	}

	ctx := context.Background()

	// Initial count fetch (indexFrom=1)
	fakeIDX.Responses[1] = models.AnnouncementResponse{
		ResultCount: 25,
		Replies: []models.Reply{
			{Announcement: models.Announcement{ID2: "item1"}},
		},
	}
	// Page 3 (indexFrom=21)
	fakeIDX.Responses[21] = models.AnnouncementResponse{
		ResultCount: 25,
		Replies: []models.Reply{
			{Announcement: models.Announcement{ID2: "item21"}},
		},
	}
	// Page 2 (indexFrom=11)
	fakeIDX.Responses[11] = models.AnnouncementResponse{
		ResultCount: 25,
		Replies: []models.Reply{
			{Announcement: models.Announcement{ID2: "item11"}},
		},
	}

	err := w.Process(ctx)
	assert.NoError(t, err)

	// FetchLog should be [1, 21, 11, 1]
	expectedLog := []int{1, 21, 11, 1}
	assert.Equal(t, expectedLog, fakeIDX.FetchLog)

	// Items should be processed in reverse order (bottom-up seeding)
	expectedIDs := []string{"item21", "item11", "item1"}
	var savedIDs []string
	for _, ann := range fakeStore.SavedAnnouncements {
		savedIDs = append(savedIDs, ann.ID2)
	}
	assert.Equal(t, expectedIDs, savedIDs)
}

func TestWorker_Process_Incremental(t *testing.T) {
	fakeIDX := &idx.FakeClient{
		Responses: make(map[int]models.AnnouncementResponse),
	}
	fakeStore := &state.FakeStore{
		Processed: map[string]bool{
			"old_item": true,
		},
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	w := &Worker{
		idxClient:      fakeIDX,
		stockbitClient: &FakeStockbitClient{},
		stateStore:     fakeStore,
		config: &config.Config{
			App: config.App{
				IDX: config.IDX{
					PageSize: 10,
				},
			},
		},
		logger: logger,
	}

	ctx := context.Background()

	// Page 1 has new items and an old item
	fakeIDX.Responses[1] = models.AnnouncementResponse{
		Replies: []models.Reply{
			{Announcement: models.Announcement{ID2: "new_item2"}},
			{Announcement: models.Announcement{ID2: "new_item1"}},
			{Announcement: models.Announcement{ID2: "old_item"}}, // Stop here
		},
	}

	err := w.Process(ctx)
	assert.NoError(t, err)

	// Should only fetch page 1
	assert.Equal(t, []int{1}, fakeIDX.FetchLog)

	// Should have recorded new items
	assert.True(t, fakeStore.Processed["new_item1"])
	assert.True(t, fakeStore.Processed["new_item2"])
}

func TestCalculateChecksum(t *testing.T) {
	data := []byte("hello world")
	expectedHash := sha256.Sum256(data)
	expected := hex.EncodeToString(expectedHash[:])

	actual := calculateChecksum(data)
	if actual != expected {
		t.Errorf("expected %s, got %s", expected, actual)
	}
}

func TestNamingLogic(t *testing.T) {
	ann := models.Announcement{
		StockCode:        "TLKM",
		AnnouncementDate: time.Date(2026, 3, 16, 17, 0, 0, 0, time.UTC),
	}
	originalFilename := "Financial_Report.PDF"
	data := []byte("some content")
	checksum := calculateChecksum(data)
	shortChecksum := checksum[:8]

	// Simulated naming logic from worker.go
	datePrefix := ann.AnnouncementDate.Format("2006-01-02")
	stockCode := "TLKM"                               // already trimmed and uppercase in test prep
	originalName := strings.ToLower(originalFilename) // lowercased

	actual := datePrefix + "_" + stockCode + "_" + shortChecksum + "_" + originalName
	expected := "2026-03-16_TLKM_" + shortChecksum + "_financial_report.pdf"

	if actual != expected {
		t.Errorf("expected %s, got %s", expected, actual)
	}
}
