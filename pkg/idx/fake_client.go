package idx

import (
	"context"
	"time"

	"github.com/alturino/bloodhound/internal/models"
)

// FakeClient implements a fake IDX client for testing
type FakeClient struct {
	Responses map[int]models.AnnouncementResponse
	FetchLog  []int
}

func (f *FakeClient) FetchAnnouncements(
	ctx context.Context,
	indexFrom int,
	dateFrom time.Time,
) (models.AnnouncementResponse, error) {
	if f.FetchLog == nil {
		f.FetchLog = []int{}
	}
	f.FetchLog = append(f.FetchLog, indexFrom)
	return f.Responses[indexFrom], nil
}

func (f *FakeClient) DownloadFile(ctx context.Context, url string) ([]byte, string, error) {
	panic("not implemented") // TODO: Implement
}
