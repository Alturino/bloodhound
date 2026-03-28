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

func TestWorker_Process_ReversePaging(t *testing.T) {
	fakeIDX := &http.FakeClient{
		Responses: make(map[int]models.AnnouncementResponse),
	}
	fakeStore := &state.FakeStore{
		Processed: make(map[string]bool),
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	w := &Worker{
		idxClient:  fakeIDX,
		stateStore: fakeStore,
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

	// Setup data for total 25 items, page size 10
	// We want to verify that it fetches: Page 1 (for count), then Page 3, 2, 1.
	
	// Page 1 initial response
	fakeIDX.Responses[1] = models.AnnouncementResponse{
		ResultCount: 25,
		Replies: []models.Reply{
			{Announcement: models.Announcement{ID2: "item1"}},
		},
	}
	
	// Page 2 response
	fakeIDX.Responses[2] = models.AnnouncementResponse{
		ResultCount: 25,
		Replies: []models.Reply{
			{Announcement: models.Announcement{ID2: "item11"}},
		},
	}
	
	// Page 3 response
	fakeIDX.Responses[3] = models.AnnouncementResponse{
		ResultCount: 25,
		Replies: []models.Reply{
			{Announcement: models.Announcement{ID2: "item21"}},
		},
	}

	err := w.Process(ctx)
	assert.NoError(t, err)

	// Verify fetch order: 1 (count), 3, 2, 1
	expectedLog := []int{1, 3, 2, 1}
	assert.Equal(t, expectedLog, fakeIDX.FetchLog)

	// Verify items processed (should be in order of discovery: item21, item11, item1)
	expectedIDs := []string{"item21", "item11", "item1"}
	var savedIDs []string
	for _, ann := range fakeStore.SavedAnnouncements {
		savedIDs = append(savedIDs, ann.ID2)
	}
	assert.Equal(t, expectedIDs, savedIDs)
}
