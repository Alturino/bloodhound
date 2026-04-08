package http

import (
	"context"

	"github.com/alturino/bloodhound/internal/models"
)

// FakeClient implements a fake IDX client for testing
type FakeClient struct {
	Responses map[int]models.AnnouncementResponse
	FetchLog  []int
}

func (f *FakeClient) FetchAnnouncements(ctx context.Context, indexFrom int) (models.AnnouncementResponse, error) {
	if f.FetchLog == nil {
		f.FetchLog = []int{}
	}
	f.FetchLog = append(f.FetchLog, indexFrom)
	return f.Responses[indexFrom], nil
}
