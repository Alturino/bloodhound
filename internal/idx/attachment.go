package idx

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/google/uuid"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type AttachmentPool interface {
	Shutdown()
}

type attachmentPool struct {
	config      *config.WorkerPool
	db          *sql.DB
	logger      *slog.Logger
	taskChan    chan AttachmentTask
	workerCount int
	ctx         context.Context
	cancel      context.CancelFunc
	tracer      trace.Tracer
	metrics     *telemetry.Metrics
	worker      AttachmentWorker
	store       AttachmentStore
}

func NewAttachmentPool(
	ctx context.Context,
	db *sql.DB,
	config *config.WorkerPool,
	logger *slog.Logger,
	tracer trace.Tracer,
	metrics *telemetry.Metrics,
	worker AttachmentWorker,
	store AttachmentStore,
) AttachmentPool {
	workerCount := config.AttachmentWorkers
	ctx, cancel := context.WithCancel(ctx)

	pool := &attachmentPool{
		config:      config,
		db:          db,
		worker:      worker,
		logger:      logger,
		tracer:      tracer,
		metrics:     metrics,
		store:       store,
		taskChan:    make(chan AttachmentTask, workerCount*2),
		ctx:         ctx,
		cancel:      cancel,
		workerCount: config.AttachmentWorkers,
	}

	pool.Start()
	return pool
}

func (p *attachmentPool) Start() {
	for i := 1; i <= p.workerCount; i++ {
		go p.workerLoop(i)
	}
	go p.poller()
}

func (p *attachmentPool) poller() {
	interval := time.Minute
	logger := p.logger.With(
		slog.String("tag", "attachmentPool.poller"),
		slog.Duration("interval", interval),
	)

	ticker := time.Tick(interval)
	for {
		select {
		case <-p.ctx.Done():
			logger.InfoContext(p.ctx, "context done, stopping poller")
			return
		case t := <-ticker:
			logger.DebugContext(
				p.ctx,
				"polling for unprocessed attachments",
				slog.Time("executed_at", t),
			)
			p.pollAndSubmit(p.ctx)
		}
	}
}

func (p *attachmentPool) pollAndSubmit(ctx context.Context) {
	ctx, span := p.tracer.Start(
		ctx,
		"attachmentPool.pollAndSubmit",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := p.logger.With(slog.String("tag", "attachmentPool.pollAndSubmit"))

	attachments, err := p.store.UnprocessedAttachments(ctx)
	if err != nil {
		return
	}

	p.metrics.AttPolledCount.Record(ctx, int64(len(attachments)))

	ctx = slogctx.Append(ctx, slog.Int("unprocessed_attachments_count", len(attachments)))
	if len(attachments) == 0 {
		logger.InfoContext(ctx, "no unprocessed attachments")
		return
	}

	ids := make([]uuid.UUID, len(attachments))
	for i, att := range attachments {
		ids[i] = att.ID
	}

	if err := p.store.ClaimAttachments(ctx, ids...); err != nil {
		return
	}

	for _, att := range attachments {
		select {
		case <-p.ctx.Done():
			logger.InfoContext(ctx, "context done, stopping")
			return
		case p.taskChan <- AttachmentTask{Ctx: ctx, Attachment: att}:
			logger.InfoContext(ctx, "submitted attachment", slog.Any("attachment", att))
			continue
		}
	}

	logger.InfoContext(ctx, "submitted attachments", slog.Int("count", len(attachments)))
}

func (p *attachmentPool) workerLoop(id int) {
	logger := p.logger.With(
		slog.String("tag", "attachmentPool.workerLoop"),
		slog.Int("worker_id", id),
	)
	for {
		select {
		case <-p.ctx.Done():
			logger.Warn("context done, stopping worker")
			return
		case task, ok := <-p.taskChan:
			ctx := slogctx.Append(
				task.Ctx,
				slog.String("attachment_id", task.Attachment.ID.String()),
				slog.Int("worker_id", id),
			)
			if !ok {
				logger.WarnContext(ctx, "task channel closed")
				return
			}
			p.processAttachment(ctx, task)
		}
	}
}

func (p *attachmentPool) processAttachment(ctx context.Context, task AttachmentTask) {
	ctx, span := p.tracer.Start(
		ctx,
		"attachmentPool.processAttachment",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("attachment_id", task.Attachment.ID.String()),
			attribute.String("idx_url", task.Attachment.IdxURL),
		),
	)
	defer span.End()

	logger := p.logger.With(slog.String("tag", "attachmentPool.processAttachment"))

	result, err := p.worker.Work(ctx, task)
	if err != nil {
		p.metrics.AttDownloaded.Add(ctx, 1, metric.WithAttributes(
			attribute.String("status", "failure"),
		))
		logger.ErrorContext(ctx, "worker error", slog.Any("error", err))
		if err := p.store.UpdateAttachmentResult(ctx, result.Attachment); err != nil {
			logger.ErrorContext(ctx, "update attachment", slog.Any("error", err))
			return
		}
		return
	}

	p.metrics.AttDownloaded.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", "success"),
	))

	if err := p.store.UpdateAttachmentResult(ctx, result.Attachment); err != nil {
		logger.ErrorContext(ctx, "update attachment result", slog.Any("error", err))
		return
	}

	logger.InfoContext(ctx, "attachment processed successfully")
}

func (p *attachmentPool) Shutdown() {
	p.cancel()
	close(p.taskChan)
	p.logger.Info("shutdown attachment pool")
}
