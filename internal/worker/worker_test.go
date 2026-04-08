package worker

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/http"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/stretchr/testify/assert"
)

type FakeStockbitClient struct {
	Response models.StockbitMarketDetectorResponse
	Err      error
}

func (f *FakeStockbitClient) FetchMarketDetector(ctx context.Context, symbol, dateFrom, dateTo string) (models.StockbitMarketDetectorResponse, error) {
	return f.Response, f.Err
}

func TestWorker_Process_InitialSeeding(t *testing.T) {
	fakeIDX := &http.FakeClient{
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
			App: config.AppConfig{
				IDX: config.IDXConfig{
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
	fakeIDX := &http.FakeClient{
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
			App: config.AppConfig{
				IDX: config.IDXConfig{
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
