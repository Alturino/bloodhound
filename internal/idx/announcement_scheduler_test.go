package idx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"

	"github.com/alturino/bloodhound/internal/config"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

func TestAnnouncementScheduler_StartShutdown(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	client := &mockClient{
		fetchFn: func(_ context.Context, _ int, _ time.Time) (AnnouncementResponse, error) {
			return AnnouncementResponse{ResultCount: 0}, nil
		},
	}

	store := &mockAnnouncementStore{
		latestFn: func(_ context.Context, _ qrm.DB) (model.Announcements, error) {
			return model.Announcements{}, errors.New("no rows")
		},
	}

	pool := NewAnnouncementPool(
		context.Background(),
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	cfg := &config.Scheduler{Interval: 100 * time.Millisecond}
	s := NewAnnouncementScheduler(
		context.Background(),
		cfg,
		10,
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
		client,
		pool,
	)

	s.Start()

	time.Sleep(200 * time.Millisecond)

	s.Shutdown()

	time.Sleep(100 * time.Millisecond)

	s.Shutdown()
}

func TestAnnouncementScheduler_ProcessCallsClient(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var fetchCount atomic.Int32

	client := &mockClient{
		fetchFn: func(_ context.Context, _ int, _ time.Time) (AnnouncementResponse, error) {
			fetchCount.Add(1)
			return AnnouncementResponse{
				ResultCount:   10,
				SearchParams:  SearchParams{PageSize: 10},
				Announcements: []Announcement{newTestAnnouncement("ann-1")},
			}, nil
		},
	}

	store := &mockAnnouncementStore{
		latestFn: func(_ context.Context, _ qrm.DB) (model.Announcements, error) {
			return model.Announcements{}, errors.New("no rows")
		},
		processedFn: func(_ context.Context, _ qrm.DB, _ ...string) (map[string]bool, error) {
			return map[string]bool{}, nil
		},
		insertFn: func(_ context.Context, _ qrm.DB, _ ...model.Announcements) error {
			return nil
		},
	}

	pool := NewAnnouncementPool(
		context.Background(),
		2,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	cfg := &config.Scheduler{Interval: 50 * time.Millisecond}
	s := NewAnnouncementScheduler(
		context.Background(),
		cfg,
		10,
		2,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
		client,
		pool,
	)

	s.Start()
	time.Sleep(300 * time.Millisecond)
	s.Shutdown()
	time.Sleep(100 * time.Millisecond)

	assert.GreaterOrEqual(t, int(fetchCount.Load()), 1)
}

func TestAnnouncementScheduler_ProcessErrorHandling(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var fetchCount atomic.Int32

	client := &mockClient{
		fetchFn: func(_ context.Context, _ int, _ time.Time) (AnnouncementResponse, error) {
			fetchCount.Add(1)
			return AnnouncementResponse{}, errors.New("network error")
		},
	}

	store := &mockAnnouncementStore{
		latestFn: func(_ context.Context, _ qrm.DB) (model.Announcements, error) {
			return model.Announcements{}, errors.New("no rows")
		},
	}

	pool := NewAnnouncementPool(
		context.Background(),
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	cfg := &config.Scheduler{Interval: 50 * time.Millisecond}
	s := NewAnnouncementScheduler(
		context.Background(),
		cfg,
		10,
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
		client,
		pool,
	)

	s.Start()
	time.Sleep(200 * time.Millisecond)
	s.Shutdown()
	time.Sleep(100 * time.Millisecond)

	assert.GreaterOrEqual(t, int(fetchCount.Load()), 1)
}

func TestAnnouncementScheduler_Pagination(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var fetchCount atomic.Int32
	var insertCount atomic.Int32

	client := &mockClient{
		fetchFn: func(_ context.Context, _ int, _ time.Time) (AnnouncementResponse, error) {
			fetchCount.Add(1)
			return AnnouncementResponse{
				ResultCount:   20,
				SearchParams:  SearchParams{PageSize: 10},
				Announcements: []Announcement{newTestAnnouncement("ann-page")},
			}, nil
		},
	}

	store := &mockAnnouncementStore{
		latestFn: func(_ context.Context, _ qrm.DB) (model.Announcements, error) {
			return model.Announcements{}, errors.New("no rows")
		},
		processedFn: func(_ context.Context, _ qrm.DB, _ ...string) (map[string]bool, error) {
			return map[string]bool{}, nil
		},
		insertFn: func(_ context.Context, _ qrm.DB, _ ...model.Announcements) error {
			insertCount.Add(1)
			return nil
		},
	}

	pool := NewAnnouncementPool(
		context.Background(),
		2,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	cfg := &config.Scheduler{Interval: 50 * time.Millisecond}
	s := NewAnnouncementScheduler(
		context.Background(),
		cfg,
		10,
		2,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
		client,
		pool,
	)

	s.Start()
	time.Sleep(300 * time.Millisecond)
	s.Shutdown()
	time.Sleep(200 * time.Millisecond)

	assert.GreaterOrEqual(t, int(fetchCount.Load()), 1)
}

func TestAnnouncementScheduler_LatestAnnouncementErrorFallsBack(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var fetchCalled atomic.Int32

	client := &mockClient{
		fetchFn: func(_ context.Context, _ int, _ time.Time) (AnnouncementResponse, error) {
			fetchCalled.Add(1)
			return AnnouncementResponse{ResultCount: 0}, nil
		},
	}

	store := &mockAnnouncementStore{
		latestFn: func(_ context.Context, _ qrm.DB) (model.Announcements, error) {
			return model.Announcements{}, errors.New("db error")
		},
	}

	pool := NewAnnouncementPool(
		context.Background(),
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	cfg := &config.Scheduler{Interval: 50 * time.Millisecond}
	s := NewAnnouncementScheduler(
		context.Background(),
		cfg,
		10,
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
		client,
		pool,
	)

	s.Start()
	time.Sleep(200 * time.Millisecond)
	s.Shutdown()
	time.Sleep(100 * time.Millisecond)

	assert.GreaterOrEqual(t, int(fetchCalled.Load()), 1)
}
