package idx

import (
	"context"
	"log/slog"
	"time"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	"github.com/alturino/bloodhound/internal/models"
)

type AttachmentPool interface {
	Shutdown()
}

type attachmentPool struct {
	config      *config.WorkerPoolConfig
	logger      *slog.Logger
	taskChan    chan model.Attachments
	workerCount int
	ctx         context.Context
	cancel      context.CancelFunc
	tracer      trace.Tracer
	worker      AttachmentWorker
	store       AttachmentStore
}

func NewAttachmentPool(
	ctx context.Context,
	config *config.WorkerPoolConfig,
	logger *slog.Logger,
	tracer trace.Tracer,
	worker AttachmentWorker,
	store AttachmentStore,
) AttachmentPool {
	workerCount := config.AttachmentWorkers
	ctx, cancel := context.WithCancel(ctx)

	pool := &attachmentPool{
		config:      config,
		worker:      worker,
		logger:      logger,
		tracer:      tracer,
		store:       store,
		taskChan:    make(chan model.Attachments, workerCount*2),
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
		case <-ticker:
			logger.DebugContext(p.ctx, "polling for unprocessed attachments")
			p.pollAndSubmit()
		}
	}
}

func (p *attachmentPool) pollAndSubmit() {
	ctx, span := p.tracer.Start(
		p.ctx,
		"attachmentPool.pollAndSubmit",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := p.logger.With(slog.String("tag", "attachmentPool.pollAndSubmit"))

	attachments, err := p.store.UnprocessedAttachments(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "get unprocessed attachments", slog.Any("error", err))
		return
	}

	if len(attachments) == 0 {
		logger.InfoContext(ctx, "no unprocessed attachments")
		return
	}

	ids := make([]string, len(attachments))
	for i, att := range attachments {
		ids[i] = att.ID.String()
	}

	if err := p.store.ClaimAttachments(ctx, ids...); err != nil {
		logger.ErrorContext(ctx, "claim attachments", slog.Any("error", err))
		return
	}

	for _, att := range attachments {
		select {
		case <-p.ctx.Done():
			logger.InfoContext(ctx, "context done, stopping")
			return
		case p.taskChan <- att:
			logger.InfoContext(ctx, "submitted attachment", slog.Any("attachment", att))
			continue
		}
	}

	logger.InfoContext(ctx, "submitted attachments", slog.Int("count", len(attachments)))
}

func (p *attachmentPool) workerLoop(id int) {
	ctx := slogctx.Append(p.ctx, slog.Int("worker_id", id))
	logger := p.logger.With(slog.String("tag", "attachmentPool.workerLoop"))
	for {
		select {
		case <-p.ctx.Done():
			logger.WarnContext(ctx, "context done, stopping worker")
			return
		case attachment, ok := <-p.taskChan:
			ctx := slogctx.Append(ctx, slog.String("attachment_id", attachment.ID.String()))
			if !ok {
				logger.WarnContext(ctx, "task channel closed")
				return
			}
			p.processAttachment(ctx, attachment)
		}
	}
}

func (p *attachmentPool) processAttachment(ctx context.Context, attachment model.Attachments) {
	ctx, span := p.tracer.Start(
		ctx,
		"attachmentPool.processAttachment",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("attachment_id", attachment.ID.String()),
			attribute.String("idx_url", attachment.IdxURL),
		),
	)
	defer span.End()

	logger := p.logger.With(slog.String("tag", "attachmentPool.processAttachment"))

	task := AttachmentTask{
		AnnouncementID: attachment.IdxAnnouncementID,
		Attachment: models.Attachment{
			PDFFilename:      attachment.Filename,
			FullSavePath:     attachment.IdxURL,
			OriginalFilename: attachment.OriginalFilename,
		},
	}

	result, err := p.worker.Work(ctx, task)
	if err != nil {
		attachment.IsDownloaded = false
		attachment.IsProcessing = false
		attachment.Error = err.Error()
		if err := p.store.UpdateAttachmentResult(ctx, attachment); err != nil {
			logger.ErrorContext(ctx, "worker error", slog.Any("error", err))
			return
		}
		return
	}

	attachment.IsDownloaded = result.IsDownloaded
	attachment.IsProcessing = false
	attachment.Checksum = result.ChecksumSHA256
	attachment.StoragePath = result.StoragePath
	attachment.Error = ""
	if err := p.store.UpdateAttachmentResult(ctx, attachment); err != nil {
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
