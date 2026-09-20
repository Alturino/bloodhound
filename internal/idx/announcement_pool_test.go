package idx

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

func TestAnnouncementPool_SubmitProcessesPage(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var insertCount atomic.Int32
	processed := make(chan struct{}, 1)

	store := &mockAnnouncementStore{
		processedFn: func(_ context.Context, _ qrm.DB, _ ...string) (map[string]bool, error) {
			return map[string]bool{}, nil
		},
		insertFn: func(_ context.Context, _ qrm.DB, _ ...model.Announcements) error {
			insertCount.Add(1)
			select {
			case processed <- struct{}{}:
			default:
			}
			return nil
		},
	}

	p := NewAnnouncementPool(
		context.Background(),
		2,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	page := &Page{
		Ctx:   context.Background(),
		Index: 0,
		Total: 1,
		Announcements: []Announcement{
			newTestAnnouncement("ann-1"),
		},
	}
	p.Submit(page)

	select {
	case <-processed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for page to be processed")
	}
	assert.GreaterOrEqual(t, int(insertCount.Load()), 1)

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}

func TestAnnouncementPool_SubmitMultiplePages(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var insertCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)

	store := &mockAnnouncementStore{
		processedFn: func(_ context.Context, _ qrm.DB, _ ...string) (map[string]bool, error) {
			return map[string]bool{}, nil
		},
		insertFn: func(_ context.Context, _ qrm.DB, _ ...model.Announcements) error {
			insertCount.Add(1)
			wg.Done()
			return nil
		},
	}

	p := NewAnnouncementPool(
		context.Background(),
		2,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	for i := 0; i < 3; i++ {
		page := &Page{
			Ctx:   context.Background(),
			Index: i,
			Total: 3,
			Announcements: []Announcement{
				newTestAnnouncement("ann-page"),
			},
		}
		p.Submit(page)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for all pages")
	}
	assert.Equal(t, 3, int(insertCount.Load()))

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}

func TestAnnouncementPool_ShutdownIdempotent(t *testing.T) {
	store := &mockAnnouncementStore{}

	p := NewAnnouncementPool(
		context.Background(),
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	time.Sleep(50 * time.Millisecond)

	p.Shutdown()
	p.Shutdown()

	time.Sleep(100 * time.Millisecond)
}

func TestAnnouncementPool_SubmitAfterShutdownDoesNotBlock(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	store := &mockAnnouncementStore{}

	p := NewAnnouncementPool(
		context.Background(),
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	time.Sleep(50 * time.Millisecond)
	p.Shutdown()
	time.Sleep(100 * time.Millisecond)

	page := &Page{
		Ctx:   context.Background(),
		Index: 0,
		Total: 1,
		Announcements: []Announcement{
			newTestAnnouncement("ann-1"),
		},
	}

	done := make(chan struct{})
	go func() {
		p.Submit(page)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Submit blocked after shutdown")
	}
}

func TestAnnouncementPool_EmptyPageNotInserted(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var insertCount atomic.Int32
	processed := make(chan struct{}, 1)

	store := &mockAnnouncementStore{
		processedFn: func(_ context.Context, _ qrm.DB, _ ...string) (map[string]bool, error) {
			return map[string]bool{}, nil
		},
		insertFn: func(_ context.Context, _ qrm.DB, _ ...model.Announcements) error {
			insertCount.Add(1)
			select {
			case processed <- struct{}{}:
			default:
			}
			return nil
		},
	}

	p := NewAnnouncementPool(
		context.Background(),
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	page := &Page{
		Ctx:           context.Background(),
		Index:         0,
		Total:         1,
		Announcements: []Announcement{},
	}
	p.Submit(page)

	select {
	case <-processed:
		t.Fatal("expected no insert for empty page")
	case <-time.After(500 * time.Millisecond):
	}
	assert.Equal(t, 0, int(insertCount.Load()))

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}

func TestAnnouncementPool_DuplicateFiltering(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var insertCount atomic.Int32
	processed := make(chan struct{}, 1)

	store := &mockAnnouncementStore{
		processedFn: func(_ context.Context, _ qrm.DB, idxIDs ...string) (map[string]bool, error) {
			result := make(map[string]bool, len(idxIDs))
			for _, id := range idxIDs {
				result[id] = id == "ann-dup"
			}
			return result, nil
		},
		insertFn: func(_ context.Context, _ qrm.DB, _ ...model.Announcements) error {
			insertCount.Add(1)
			select {
			case processed <- struct{}{}:
			default:
			}
			return nil
		},
	}

	p := NewAnnouncementPool(
		context.Background(),
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		&mockAttachmentStore{},
	)

	page := &Page{
		Ctx:   context.Background(),
		Index: 0,
		Total: 1,
		Announcements: []Announcement{
			newTestAnnouncement("ann-dup"),
			newTestAnnouncement("ann-new"),
		},
	}
	p.Submit(page)

	select {
	case <-processed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for page processing")
	}

	assert.Equal(t, 1, int(insertCount.Load()))

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}

func TestAnnouncementPool_AttachmentInsertion(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	attProcessed := make(chan struct{}, 1)

	store := &mockAnnouncementStore{
		processedFn: func(_ context.Context, _ qrm.DB, _ ...string) (map[string]bool, error) {
			return map[string]bool{}, nil
		},
		insertFn: func(_ context.Context, _ qrm.DB, _ ...model.Announcements) error {
			return nil
		},
	}

	attStore := &mockAttachmentStore{
		insertFn: func(_ context.Context, _ qrm.DB, _ ...model.Attachments) error {
			select {
			case attProcessed <- struct{}{}:
			default:
			}
			return nil
		},
	}

	p := NewAnnouncementPool(
		context.Background(),
		1,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		attStore,
	)

	page := &Page{
		Ctx:   context.Background(),
		Index: 0,
		Total: 1,
		Announcements: []Announcement{
			newTestAnnouncement("ann-1"),
		},
	}
	p.Submit(page)

	select {
	case <-attProcessed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for attachment insert")
	}

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}
