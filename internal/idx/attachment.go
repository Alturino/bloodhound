package idx

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/google/uuid"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/constants"
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
	interval := p.config.Scheduler.Interval
	logger := p.logger.With(
		slog.String("tag", "idx.attachmentPool.poller"),
		slog.Duration(constants.Interval, interval),
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
				slog.Time(constants.ExecutedAt, t),
			)
			p.pollAndSubmit(p.ctx)
		}
	}
}

func (p *attachmentPool) pollAndSubmit(ctx context.Context) {
	ctx, span := p.tracer.Start(
		ctx,
		"idx.attachmentPool.pollAndSubmit",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	span.AddEvent("polling and submitting")

	logger := p.logger.With(slog.String("tag", "idx.attachmentPool.pollAndSubmit"))

	attachments, err := p.store.UnprocessedAttachments(ctx)
	if err != nil {
		return
	}
	if len(attachments) == 0 {
		return
	}
	p.metrics.AttPolledCount.Record(ctx, int64(len(attachments)))

	ctx = slogctx.Append(ctx, slog.Int(constants.UnprocessedAttachmentsCount, len(attachments)))
	if len(attachments) == 0 {
		logger.InfoContext(ctx, "no unprocessed attachments")
		span.AddEvent("no unprocessed attachments")
		return
	}

	ids := make([]uuid.UUID, len(attachments))
	for i, att := range attachments {
		ids[i] = att.ID
	}

	attachments, err = p.store.ClaimAttachments(ctx, ids...)
	if err != nil {
		return
	}

	for _, att := range attachments {
		select {
		case <-p.ctx.Done():
			logger.InfoContext(ctx, "context done, stopping")
			span.AddEvent("context done, stopping")
			return
		case p.taskChan <- AttachmentTask{Ctx: ctx, Attachment: &att}:
			continue
		}
	}

	logger.InfoContext(ctx, "submitted attachments", slog.Int(constants.Count, len(attachments)))
	span.AddEvent("submitted attachments")
}

func (p *attachmentPool) workerLoop(id int) {
	logger := p.logger.With(
		slog.String("tag", "idx.attachmentPool.workerLoop"),
		slog.Int(constants.WorkerID, id),
	)
	for {
		select {
		case <-p.ctx.Done():
			logger.Warn("context done, stopping worker")
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

func (p *attachmentPool) processAttachment(ctx context.Context, task AttachmentTask) {
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
	result, err := p.worker.Work(ctx, &task)
	if err != nil {
		p.metrics.AttFailedDownload.Add(ctx, 1)
		logger.ErrorContext(ctx, "worker error", slog.Any("error", err))
		if err := p.store.UpdateAttachmentResult(ctx, result.Attachment); err != nil {
			logger.ErrorContext(ctx, "update attachment", slog.Any("error", err))
			return
		}
		return
	}
	p.metrics.AttDownloaded.Add(ctx, 1)

	if err := p.store.UpdateAttachmentResult(ctx, result.Attachment); err != nil {
		logger.ErrorContext(ctx, "update attachment result", slog.Any("error", err))
		return
	}

	logger.InfoContext(ctx, "attachment processed successfully")
	span.AddEvent("attachment processed successfully")
}

func (p *attachmentPool) Shutdown() {
	p.cancel()
	close(p.taskChan)
	p.logger.Info("shutdown attachment pool")
}
