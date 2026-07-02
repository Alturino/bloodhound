package idx

import (
	"context"
	"log/slog"
	"sync"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type attachmentPool struct {
	logger       *slog.Logger
	taskChan     chan *AttachmentTask
	workerCount  int
	ctx          context.Context
	cancel       context.CancelFunc
	tracer       trace.Tracer
	metrics      *telemetry.Metrics
	worker       AttachmentWorker
	store        AttachmentStore
	shutdownOnce sync.Once
}

func NewAttachmentPool(
	ctx context.Context,
	cfg *config.WorkerPool,
	logger *slog.Logger,
	tracer trace.Tracer,
	metrics *telemetry.Metrics,
	worker AttachmentWorker,
	store AttachmentStore,
) *attachmentPool {
	ctx, cancel := context.WithCancel(ctx)

	pool := &attachmentPool{
		worker:      worker,
		logger:      logger,
		tracer:      tracer,
		metrics:     metrics,
		store:       store,
		taskChan:    make(chan *AttachmentTask, cfg.AttachmentWorkers*2),
		ctx:         ctx,
		cancel:      cancel,
		workerCount: cfg.AttachmentWorkers,
	}
	pool.Start()
	return pool
}

func (p *attachmentPool) Start() {
	for i := 1; i <= p.workerCount; i++ {
		go p.workerLoop(i)
	}
}

func (p *attachmentPool) Submit(task *AttachmentTask) {
	select {
	case <-p.ctx.Done():
		return
	case p.taskChan <- task:
	}
}

func (p *attachmentPool) workerLoop(id int) {
	logger := p.logger.With(
		slog.String("tag", "idx.attachmentPool.workerLoop"),
		slog.Int(constants.WorkerID, id),
	)
	for {
		select {
		case <-p.ctx.Done():
			logger.Info("context done, stopping worker")
			return
		case task, ok := <-p.taskChan:
			ctx := slogctx.Append(
				task.Ctx,
				slog.Any(constants.Attachment, task.Attachment),
				slog.Int(constants.WorkerID, id),
			)
			if !ok {
				logger.WarnContext(ctx, "task channel closed")
				return
			}
			p.processAttachment(ctx, task)
		}
	}
}

func (p *attachmentPool) processAttachment(ctx context.Context, task *AttachmentTask) {
	ctx, span := p.tracer.Start(
		ctx,
		"idx.attachmentPool.processAttachment",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String(constants.AttachmentID, task.Attachment.ID.String()),
			attribute.String(constants.IdxURL, task.Attachment.IdxURL),
		),
	)
	defer span.End()

	logger := p.logger.With(slog.String("tag", "attachmentPool.processAttachment"))

	logger.DebugContext(ctx, "processing attachment")
	span.AddEvent("processing attachment")
	result, err := p.worker.Work(ctx, task)
	if err != nil {
		p.metrics.AttFailedDownload.Add(ctx, 1)
		logger.ErrorContext(ctx, "worker error", slog.Any("error", err))
		if err := p.store.UpdateAttachmentResult(ctx, &result.Attachment); err != nil {
			logger.ErrorContext(ctx, "update attachment", slog.Any("error", err))
			return
		}
		return
	}
	p.metrics.AttDownloaded.Add(ctx, 1)
	if err := p.store.UpdateAttachmentResult(ctx, &result.Attachment); err != nil {
		logger.ErrorContext(ctx, "update attachment result", slog.Any("error", err))
		return
	}
	logger.InfoContext(ctx, "attachment processed")
	span.AddEvent("attachment processed")
}

func (p *attachmentPool) Shutdown() {
	p.shutdownOnce.Do(func() {
		p.cancel()
		close(p.taskChan)
		p.logger.Info("shutdown attachment pool")
	})
}
