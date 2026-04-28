package idx

import (
	"context"
	"errors"
	"log/slog"

	slogcontext "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/models"
)

// HandleAttachmentsArgs holds announcement for attachment processing
type HandleAttachmentsArgs struct {
	models.Announcement
}

// AttachmentPool processes attachments in parallel using worker pool pattern
type AttachmentPool interface {
	Pool[AttachmentTask]
	// Handle processes all attachments for a given announcement
	Handle(ctx context.Context, args HandleAttachmentsArgs) ([]AttachmentResult, error)
}

type attachmentPool struct {
	workerCount int
	taskChan    chan AttachmentTask
	resultChan  chan AttachmentResult
	logger      *slog.Logger
	tracer      trace.Tracer
	processor   AttachmentProcessor
}

func NewAttachmentPool(
	ctx context.Context,
	workerCount int,
	logger *slog.Logger,
	tracer trace.Tracer,
	processor AttachmentProcessor,
) AttachmentPool {
	if workerCount > maxWorkers {
		workerCount = maxWorkers
	}
	if workerCount < 1 {
		workerCount = 1
	}

	ap := &attachmentPool{
		workerCount: workerCount,
		taskChan:    make(chan AttachmentTask, workerCount*2),
		resultChan:  make(chan AttachmentResult, workerCount*2),
		processor:   processor,
		logger:      logger,
	}
	ap.Start(ctx)
	return ap
}

func (p *attachmentPool) Start(ctx context.Context) {
	for i := 1; i <= p.workerCount; i++ {
		go p.worker(ctx, i)
	}
}

func (p attachmentPool) Submit(ctx context.Context, task AttachmentTask) {
	ctx, span := p.tracer.Start(ctx, "AttachmentPool.Submit")
	defer span.End()

	logger := p.logger.With(
		slog.String("tag", "AttachmentPool.Submit"),
		slog.String("announcement_id", task.AnnouncementID),
		slog.String("filename", task.Attachment.OriginalFilename),
	)

	logger.InfoContext(ctx, "submitting attachment task")
	span.AddEvent("submitting attachment task")
	p.taskChan <- task
	span.AddEvent("submitted attachment task")
	logger.InfoContext(ctx, "submitted attachment task")
}

func (p attachmentPool) worker(ctx context.Context, id int) {
	logger := p.logger.With()
	ctx = slogcontext.With(ctx,
		slog.Int("worker_id", id),
		slog.String("worker_type", "attachment"),
		slog.String("tag", "AttachmentPool.worker"),
	)
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker received shutdown signal, exiting")
			return
		case task, ok := <-p.taskChan:
			if !ok {
				logger.Info("task channel closed, worker exiting")
				return
			}

			ctx := slogcontext.With(
				ctx,
				slog.Int("worker_id", id),
				slog.String("announcement_id", task.AnnouncementID),
				slog.String("filename", task.Attachment.OriginalFilename),
			)
			if err := p.processor.ProcessAttachment(ctx, task); err != nil {
				p.resultChan <- AttachmentResult{
					Index:           task.Index,
					WorkerID:        id,
					TotalAttachment: task.TotalAttachment,
					Filename:        task.Attachment.OriginalFilename,
					AnnouncementID:  task.AnnouncementID,
					Err:             err,
				}
				continue
			}
			p.resultChan <- AttachmentResult{
				Index:           task.Index,
				WorkerID:        id,
				TotalAttachment: task.TotalAttachment,
				Filename:        task.Attachment.OriginalFilename,
				AnnouncementID:  task.AnnouncementID,
				Err:             nil,
			}
		}
	}
}

func (p attachmentPool) Handle(
	ctx context.Context,
	arg HandleAttachmentsArgs,
) ([]AttachmentResult, error) {
	ctx, span := p.tracer.Start(
		ctx,
		"AttachmentPool.Handle",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(),
	)
	defer span.End()

	ctx = slogcontext.With(ctx, slog.String("tag", "AttachmentPool.Handle"))
	logger := p.logger.With()

	completed := 0

	for i, attachment := range arg.Attachments {
		ctx := slogcontext.Append(
			ctx,
			slog.String("announcement_title", arg.AnnouncementTitle),
			slog.String("announcement_id", arg.ID2),
			slog.Time("announcement_date", arg.Date),
			slog.Int("attachment_item", i+1),
			slog.String("stock_code", arg.StockCode),
			slog.String("filename", attachment.OriginalFilename),
		)
		go func(ctx context.Context) {
			logger.DebugContext(ctx, "sending to pool")
			p.Submit(
				ctx,
				AttachmentTask{
					TotalAttachment:   len(arg.Attachments),
					AnnouncementID:    arg.ID2,
					Index:             i + 1,
					AnnouncementTitle: arg.AnnouncementTitle,
					StockCode:         arg.StockCode,
					Date:              arg.Date,
					Attachment:        attachment,
				},
			)
			logger.InfoContext(ctx, "sent to pool")
		}(ctx)
	}

	results := make(map[string][]AttachmentResult, 10)
	errs := make([]error, 0, 10)
loop:
	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "receive ctx.done, stopping", slog.Any("error", ctx.Err()))
			return []AttachmentResult{}, ctx.Err()
		case result, ok := <-p.resultChan:
			results[result.AnnouncementID] = append(results[result.AnnouncementID], result)
			logger := logger.With(
				slog.Int("result_index", result.Index),
				slog.String("announcement_id", result.AnnouncementID),
				slog.Any("error", result.Err),
			)
			if !ok {
				err := errors.New("result channel closed")
				logger.ErrorContext(ctx, "could not retrieve result, channel closed")
				return []AttachmentResult{}, err
			}
			if result.Err != nil {
				logger.ErrorContext(ctx, "announcement processing error")
				errs = append(errs, result.Err)
				continue
			}
			logger.InfoContext(ctx, "announcement processed successfully")
			completed++
			if completed >= len(arg.Attachments) {
				logger.InfoContext(ctx, "announcement page processed successfully")
				break loop
			}
		}
	}

	return results[arg.ID2], errors.Join(errs...)
}

func (p *attachmentPool) Shutdown() {
	p.logger.Info("shutting down attachment pool")
	close(p.taskChan)
	close(p.resultChan)
}
