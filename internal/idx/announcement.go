package idx

import (
	"context"
	"errors"
	"log/slog"

	slogcontext "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/models"
)

// AnnouncementTask represents a single announcement processing task
type AnnouncementTask struct {
	Page         int
	Index        int
	Announcement models.Announcement
}

type AnnouncementResult struct {
	Page           int
	Index          int
	AnnouncementID string
	Err            error
}

// AnnouncementPool processes announcements in parallel using worker pool pattern
type AnnouncementPool interface {
	// Submit adds a task to the worker pool
	Submit(ctx context.Context, task AnnouncementTask)
	// ProcessPage processes all announcements for a given page
	ProcessPage(
		ctx context.Context,
		page int,
		announcements []models.Announcement,
	) ([]AnnouncementResult, error)
	// Shutdown stops the worker pool and releases resources
	Shutdown()
}

type announcementPool struct {
	workerCount int
	taskChan    chan AnnouncementTask
	resultChan  chan AnnouncementResult
	logger      *slog.Logger
	tracer      trace.Tracer
	wpConfig    config.WorkerPoolConfig
	processor   AnnouncementProcessor
}

const maxWorkers = 4

func NewAnnouncementPool(
	ctx context.Context,
	config config.WorkerPoolConfig,
	logger *slog.Logger,
	tracer trace.Tracer,
	processor AnnouncementProcessor,
) AnnouncementPool {
	workerCount := config.AnnouncementWorkers
	if workerCount > maxWorkers {
		workerCount = maxWorkers
	}
	if workerCount < 1 {
		workerCount = 2
	}

	ap := &announcementPool{
		workerCount: workerCount,
		taskChan:    make(chan AnnouncementTask, workerCount*10),
		resultChan:  make(chan AnnouncementResult, workerCount*10),
		wpConfig:    config,
		logger:      logger,
		processor:   processor,
		tracer:      tracer,
	}
	ap.Start(ctx)
	return ap
}

func (p *announcementPool) Start(ctx context.Context) {
	for i := 1; i <= p.workerCount; i++ {
		go p.worker(ctx, i)
	}
}

func (p *announcementPool) Submit(ctx context.Context, task AnnouncementTask) {
	ctx, span := p.tracer.Start(ctx, "AnnouncementPool.Submit")
	defer span.End()

	logger := p.logger.With(slog.String("tag", "AnnouncementPool.Submit"))

	logger.DebugContext(ctx, "submitting announcement task")
	span.AddEvent("submitting announcement task")
	p.taskChan <- task
	span.AddEvent("submitted announcement task")
	logger.InfoContext(ctx, "submitted announcement task")
}

func (p *announcementPool) ProcessPage(
	ctx context.Context,
	page int,
	announcements []models.Announcement,
) ([]AnnouncementResult, error) {
	ctx, span := p.tracer.Start(ctx, "AnnouncementPool.ProcessPage")
	defer span.End()

	ctx = slogcontext.Append(
		ctx,
		slog.Int("page", page),
		slog.String("tag", "AnnouncementPool.ProcessPage"),
	)
	logger := p.logger.With()

	completed := 0

	for i, ann := range announcements {
		ctx := slogcontext.Append(ctx, slog.Int("page_item", i))
		go func(ctx context.Context) {
			p.Submit(ctx, AnnouncementTask{
				Page:         page,
				Index:        i,
				Announcement: ann,
			})
		}(ctx)
	}

	results := make(map[int][]AnnouncementResult, 10)
	errs := make([]error, 0, 10)
loop:
	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "receive ctx.done, stopping", slog.Any("error", ctx.Err()))
			return []AnnouncementResult{}, ctx.Err()
		case result, ok := <-p.resultChan:
			results[result.Page] = append(results[result.Page], result)
			logger := logger.With(
				slog.Int("result_page", result.Page),
				slog.Int("result_index", result.Index),
				slog.Int("result_received", len(results[result.Page])),
				slog.String("announcement_id", result.AnnouncementID),
				slog.Any("error", result.Err),
			)
			if !ok {
				err := errors.New("result channel closed")
				logger.ErrorContext(ctx, "could not retrieve result, channel closed")
				return []AnnouncementResult{}, err
			}
			if result.Err != nil {
				logger.ErrorContext(ctx, "announcement processing error")
				errs = append(errs, result.Err)
				continue
			}
			logger.InfoContext(ctx, "announcement processed successfully")
			completed++
			if completed >= len(announcements) {
				logger.InfoContext(ctx, "announcement page processed successfully")
				break loop
			}
		}
	}

	return results[page], errors.Join(errs...)
}

func (p *announcementPool) worker(ctx context.Context, id int) {
	logger := p.logger.With(
		slog.String("tag", "AnnouncementPool.worker"),
		slog.Int("worker_id", id),
	)
	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "receive ctx.done, stopping", slog.Any("error", ctx.Err()))
			return
		case task, ok := <-p.taskChan:
			if !ok {
				logger.WarnContext(ctx, "could not retrieve task, channel closed")
				continue
			}

			ctx := slogcontext.Append(
				ctx,
				slog.Int("worker_id", id),
				slog.Int("page", task.Page),
				slog.Int("index", task.Index),
				slog.String("announcement_id", task.Announcement.ID2),
			)

			logger.InfoContext(ctx, "processing announcement")
			if err := p.processor.ProcessAnnouncement(ctx, task.Announcement); err != nil {
				logger.ErrorContext(ctx, "process announcement", slog.Any("error", err))
				p.resultChan <- AnnouncementResult{
					AnnouncementID: task.Announcement.ID2,
					Err:            err,
					Page:           task.Page,
					Index:          task.Index,
				}
				continue
			}
			p.resultChan <- AnnouncementResult{
				AnnouncementID: task.Announcement.ID2,
				Err:            nil,
				Page:           task.Page,
				Index:          task.Index,
			}
			logger.InfoContext(ctx, "finished processing announcement")
		}
	}
}

func (p *announcementPool) Shutdown() {
	p.logger.Info("shutting down announcement pool")
	close(p.taskChan)
	close(p.resultChan)
}
