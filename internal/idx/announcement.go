package idx

import (
	"context"
	"log/slog"
	"sync"

	slogcontext "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/models"
)

type announcementPool struct {
	workerCount int
	taskChan    chan *AnnouncementTask
	resultChan  chan AnnouncementResult
	logger      *slog.Logger
	tracer      trace.Tracer
	processor   AnnouncementProcessor
	wg          sync.WaitGroup
	config      config.WorkerPoolConfig
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
		taskChan:    make(chan *AnnouncementTask, workerCount*10),
		resultChan:  make(chan AnnouncementResult, workerCount*10),
		config:      config,
		logger:      logger,
		processor:   processor,
		tracer:      tracer,
		wg:          sync.WaitGroup{},
	}
	ap.Start(ctx)
	return ap
}

func (p *announcementPool) Start(ctx context.Context) {
	for i := 1; i <= p.workerCount; i++ {
		go p.worker(ctx, i)
	}
}

func (p *announcementPool) Submit(ctx context.Context, task *AnnouncementTask) {
	ctx, span := p.tracer.Start(ctx, "AnnouncementPool.Submit")
	defer span.End()

	logger := p.logger.With(slog.String("tag", "AnnouncementPool.Submit"))

	logger.DebugContext(ctx, "submitting announcement task")
	span.AddEvent("submitting announcement task")
	p.wg.Add(1)
	task.Ctx = ctx
	p.taskChan <- task
	span.AddEvent("submitted announcement task")
	logger.InfoContext(ctx, "submitted announcement task")
}

func (p *announcementPool) Process(
	ctx context.Context,
	page int,
	announcements []models.Announcement,
) {
	ctx, span := p.tracer.Start(ctx, "AnnouncementPool.ProcessPage")
	defer span.End()

	ctx = slogcontext.Append(ctx,
		slog.Int("page", page),
		slog.String("tag", "AnnouncementPool.ProcessPage"),
	)
	logger := p.logger.With()
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogcontext.Append(ctx, slog.Any("announcements", announcements))
	}

	logger.DebugContext(ctx, "processing announcements")
	var wg sync.WaitGroup
	for i, ann := range announcements {
		ctx := slogcontext.Append(ctx, slog.Int("announcement_item", i+1))
		wg.Go(func() {
			func(ctx context.Context) {
				p.Submit(ctx, &AnnouncementTask{
					Page:             page,
					AnnouncementItem: i + 1,
					Announcement:     ann,
				})
			}(ctx)
		})
	}
	logger.DebugContext(ctx, "waiting for announcements to process")
	wg.Wait()
	logger.DebugContext(ctx, "finished waiting, announcements processed")
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
			p.handleTask(task.Ctx, task)
		}
	}
}

func (p *announcementPool) handleTask(ctx context.Context, task *AnnouncementTask) {
	ctx, span := p.tracer.Start(ctx, "AnnouncementPool.handleTask")
	defer span.End()
	defer p.wg.Done()

	ctx = slogcontext.Append(
		ctx,
		slog.String("tag", "idx.announcementPool.handleTask"),
		slog.String("announcement_id", task.Announcement.ID2),
		slog.String("announcement_title", task.Announcement.AnnouncementTitle),
		slog.Int("announcement_item", task.AnnouncementItem),
		slog.Int("page", task.Page),
	)
	logger := p.logger.With()

	logger.InfoContext(ctx, "processing announcement")
	result := AnnouncementResult{
		Page:             task.Page,
		AnnouncementID:   task.Announcement.ID2,
		AnnouncementItem: task.AnnouncementItem,
	}
	if err := p.processor.Process(ctx, task); err != nil {
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		result.Err = err
		p.resultChan <- result
		return
	}
	p.resultChan <- result
	logger.InfoContext(ctx, "successfully processing announcement")
}

func (p *announcementPool) Shutdown() {
	p.logger.Info("shutting down announcement pool")
	p.wg.Wait()
	close(p.taskChan)
	close(p.resultChan)
}
