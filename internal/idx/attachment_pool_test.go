package idx

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"

	"github.com/alturino/bloodhound/internal/config"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

func TestAttachmentPool_SubmitProcessesTask(t *testing.T) {
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

	p := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 2},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		&mockAttachmentStore{},
	)

	task := &AttachmentTask{
		Ctx:        context.Background(),
		Attachment: newTestAttachment(uuid.New()),
	}
	p.Submit(task)

	select {
	case <-processed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for task to be processed")
	}
	assert.Equal(t, 1, int(workCount.Load()))

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}

func TestAttachmentPool_SubmitMultipleTasks(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var workCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(5)

	worker := &mockAttachmentWorker{
		workFn: func(_ context.Context, task *AttachmentTask) (AttachmentResult, error) {
			workCount.Add(1)
			wg.Done()
			return AttachmentResult{AttachmentTask: task}, nil
		},
	}

	p := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 3},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		&mockAttachmentStore{},
	)

	for i := 0; i < 5; i++ {
		task := &AttachmentTask{
			Ctx:        context.Background(),
			Attachment: newTestAttachment(uuid.New()),
		}
		p.Submit(task)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for all tasks")
	}
	assert.Equal(t, 5, int(workCount.Load()))

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}

func TestAttachmentPool_ShutdownIdempotent(t *testing.T) {
	worker := &mockAttachmentWorker{}

	p := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		&mockAttachmentStore{},
	)

	time.Sleep(50 * time.Millisecond)

	p.Shutdown()
	p.Shutdown()

	time.Sleep(100 * time.Millisecond)
}

func TestAttachmentPool_SubmitAfterShutdownDoesNotBlock(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	worker := &mockAttachmentWorker{}

	p := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		&mockAttachmentStore{},
	)

	time.Sleep(50 * time.Millisecond)
	p.Shutdown()
	time.Sleep(100 * time.Millisecond)

	task := &AttachmentTask{
		Ctx:        context.Background(),
		Attachment: newTestAttachment(uuid.New()),
	}

	done := make(chan struct{})
	go func() {
		p.Submit(task)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Submit blocked after shutdown")
	}
}

func TestAttachmentPool_WorkerErrorUpdatesStore(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var updateCount atomic.Int32
	processed := make(chan struct{}, 1)

	worker := &mockAttachmentWorker{
		workFn: func(_ context.Context, task *AttachmentTask) (AttachmentResult, error) {
			task.Attachment.IsDownloaded = false
			task.Attachment.Error = "download failed"
			return AttachmentResult{AttachmentTask: task}, errors.New("download failed")
		},
	}

	store := &mockAttachmentStore{
		updateFn: func(_ context.Context, att *model.Attachments) error {
			updateCount.Add(1)
			select {
			case processed <- struct{}{}:
			default:
			}
			return nil
		},
	}

	p := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		store,
	)

	task := &AttachmentTask{
		Ctx:        context.Background(),
		Attachment: newTestAttachment(uuid.New()),
	}
	p.Submit(task)

	select {
	case <-processed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for error handling")
	}

	assert.Equal(t, 1, int(updateCount.Load()))

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}

func TestAttachmentPool_WorkerSuccessUpdatesStore(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var updatedAttachment *model.Attachments
	processed := make(chan struct{}, 1)

	worker := &mockAttachmentWorker{
		workFn: func(_ context.Context, task *AttachmentTask) (AttachmentResult, error) {
			task.Attachment.IsDownloaded = true
			return AttachmentResult{AttachmentTask: task}, nil
		},
	}

	store := &mockAttachmentStore{
		updateFn: func(_ context.Context, att *model.Attachments) error {
			updatedAttachment = att
			select {
			case processed <- struct{}{}:
			default:
			}
			return nil
		},
	}

	p := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		store,
	)

	task := &AttachmentTask{
		Ctx:        context.Background(),
		Attachment: newTestAttachment(uuid.New()),
	}
	p.Submit(task)

	select {
	case <-processed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for update")
	}

	assert.NotNil(t, updatedAttachment)
	assert.True(t, updatedAttachment.IsDownloaded)

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}

func TestAttachmentPool_WorkerConcurrency(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32
	var completed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(6)

	worker := &mockAttachmentWorker{
		workFn: func(_ context.Context, task *AttachmentTask) (AttachmentResult, error) {
			cur := currentConcurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			currentConcurrent.Add(-1)
			completed.Add(1)
			wg.Done()
			return AttachmentResult{AttachmentTask: task}, nil
		},
	}

	p := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 3},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		&mockAttachmentStore{},
	)

	for i := 0; i < 6; i++ {
		task := &AttachmentTask{
			Ctx:        context.Background(),
			Attachment: newTestAttachment(uuid.New()),
		}
		p.Submit(task)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for concurrency test")
	}

	assert.Equal(t, 6, int(completed.Load()))
	assert.GreaterOrEqual(t, int(maxConcurrent.Load()), 2,
		"expected at least 2 concurrent workers")

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}

func TestAttachmentPool_StoreUpdateErrorDoesNotPanic(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	processed := make(chan struct{}, 1)

	worker := &mockAttachmentWorker{
		workFn: func(_ context.Context, task *AttachmentTask) (AttachmentResult, error) {
			return AttachmentResult{AttachmentTask: task}, nil
		},
	}

	store := &mockAttachmentStore{
		updateFn: func(_ context.Context, att *model.Attachments) error {
			select {
			case processed <- struct{}{}:
			default:
			}
			return errors.New("db connection lost")
		},
	}

	p := NewAttachmentPool(
		context.Background(),
		&config.WorkerPool{AttachmentWorkers: 1},
		newNoopLogger(),
		newNoopTracer(),
		newTestMetricsProvider(t),
		worker,
		store,
	)

	task := &AttachmentTask{
		Ctx:        context.Background(),
		Attachment: newTestAttachment(uuid.New()),
	}
	p.Submit(task)

	select {
	case <-processed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for store update error")
	}

	p.Shutdown()
	time.Sleep(50 * time.Millisecond)
}
