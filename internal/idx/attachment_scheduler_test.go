package idx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"

	"github.com/alturino/bloodhound/internal/config"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

func TestAttachmentScheduler_StartShutdown(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	store := &mockAttachmentStore{
		unprocessedFn: func(_ context.Context) ([]model.Attachments, error) {
			return nil, nil
		},
	}

	worker := &mockAttachmentWorker{}

	pool := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		store,
	)

	cfg := &config.Scheduler{Interval: 100 * time.Millisecond}
	s := NewAttachmentScheduler(
		context.Background(),
		cfg,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		pool,
	)

	time.Sleep(200 * time.Millisecond)

	s.Shutdown()
	time.Sleep(100 * time.Millisecond)

	s.Shutdown()
}

func TestAttachmentScheduler_PollAndSubmit(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var workCount atomic.Int32
	processed := make(chan struct{}, 1)

	worker := &mockAttachmentWorker{
		workFn: func(_ context.Context, task *AttachmentTask) (AttachmentResult, error) {
			workCount.Add(1)
			select {
			case processed <- struct{}{}:
			default:
			}
			return AttachmentResult{AttachmentTask: task}, nil
		},
	}

	attID := uuid.New()
	store := &mockAttachmentStore{
		unprocessedFn: func(_ context.Context) ([]model.Attachments, error) {
			return []model.Attachments{newTestAttachment(attID)}, nil
		},
		claimFn: func(_ context.Context, id ...uuid.UUID) ([]model.Attachments, error) {
			attachments := make([]model.Attachments, len(id))
			for i, uid := range id {
				attachments[i] = newTestAttachment(uid)
				attachments[i].IsProcessing = true
			}
			return attachments, nil
		},
		updateFn: func(_ context.Context, att *model.Attachments) error {
			return nil
		},
	}

	pool := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		store,
	)

	cfg := &config.Scheduler{Interval: 50 * time.Millisecond}
	s := NewAttachmentScheduler(
		context.Background(),
		cfg,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		pool,
	)

	select {
	case <-processed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for attachment to be processed")
	}

	assert.GreaterOrEqual(t, int(workCount.Load()), 1)

	s.Shutdown()
	time.Sleep(100 * time.Millisecond)
}

func TestAttachmentScheduler_NoUnprocessedAttachments(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var workCount atomic.Int32

	worker := &mockAttachmentWorker{
		workFn: func(_ context.Context, task *AttachmentTask) (AttachmentResult, error) {
			workCount.Add(1)
			return AttachmentResult{AttachmentTask: task}, nil
		},
	}

	store := &mockAttachmentStore{
		unprocessedFn: func(_ context.Context) ([]model.Attachments, error) {
			return nil, nil
		},
	}

	pool := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		store,
	)

	cfg := &config.Scheduler{Interval: 50 * time.Millisecond}
	s := NewAttachmentScheduler(
		context.Background(),
		cfg,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		pool,
	)

	time.Sleep(200 * time.Millisecond)
	s.Shutdown()
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 0, int(workCount.Load()))
}

func TestAttachmentScheduler_PollErrorDoesNotPanic(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var pollCount atomic.Int32

	worker := &mockAttachmentWorker{}

	store := &mockAttachmentStore{
		unprocessedFn: func(_ context.Context) ([]model.Attachments, error) {
			pollCount.Add(1)
			return nil, errors.New("db connection lost")
		},
	}

	pool := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		store,
	)

	cfg := &config.Scheduler{Interval: 50 * time.Millisecond}
	s := NewAttachmentScheduler(
		context.Background(),
		cfg,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		pool,
	)

	time.Sleep(200 * time.Millisecond)
	s.Shutdown()
	time.Sleep(100 * time.Millisecond)

	assert.GreaterOrEqual(t, int(pollCount.Load()), 1)
}

func TestAttachmentScheduler_ClaimErrorDoesNotPanic(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var claimCount atomic.Int32

	worker := &mockAttachmentWorker{}

	attID := uuid.New()
	store := &mockAttachmentStore{
		unprocessedFn: func(_ context.Context) ([]model.Attachments, error) {
			return []model.Attachments{newTestAttachment(attID)}, nil
		},
		claimFn: func(_ context.Context, id ...uuid.UUID) ([]model.Attachments, error) {
			claimCount.Add(1)
			return nil, errors.New("claim failed")
		},
	}

	pool := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		store,
	)

	cfg := &config.Scheduler{Interval: 50 * time.Millisecond}
	s := NewAttachmentScheduler(
		context.Background(),
		cfg,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		pool,
	)

	time.Sleep(200 * time.Millisecond)
	s.Shutdown()
	time.Sleep(100 * time.Millisecond)

	assert.GreaterOrEqual(t, int(claimCount.Load()), 1)
}

func TestAttachmentScheduler_ContextCancellation(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var pollCount atomic.Int32

	worker := &mockAttachmentWorker{}

	store := &mockAttachmentStore{
		unprocessedFn: func(_ context.Context) ([]model.Attachments, error) {
			pollCount.Add(1)
			return nil, nil
		},
	}

	pool := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		store,
	)

	cfg := &config.Scheduler{Interval: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	s := NewAttachmentScheduler(
		ctx,
		cfg,
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		store,
		pool,
	)

	time.Sleep(200 * time.Millisecond)
	cancel()
	s.Shutdown()
	time.Sleep(100 * time.Millisecond)

	assert.GreaterOrEqual(t, int(pollCount.Load()), 1)
}
