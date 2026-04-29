package idx

import (
	"context"
	"log/slog"
	"sync"

	slogcontext "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
)

// Attachment processes attachments in parallel using worker pool pattern
type Attachment interface {
	Handle(ctx context.Context, args *AnnouncementTask)
}

type attachmentPool struct {
	config    *config.WorkerPoolConfig
	logger    *slog.Logger
	tracer    trace.Tracer
	processor AttachmentProcessor
}

func NewAttachmentPool(
	ctx context.Context,
	config *config.WorkerPoolConfig,
	logger *slog.Logger,
	tracer trace.Tracer,
	processor AttachmentProcessor,
) Attachment {
	return &attachmentPool{
		config:    config,
		processor: processor,
		logger:    logger,
		tracer:    tracer,
	}
}

func (p attachmentPool) Handle(
	ctx context.Context,
	arg *AnnouncementTask,
) {
	ctx, span := p.tracer.Start(
		ctx,
		"AttachmentPool.Handle",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := p.logger.With(slog.String("tag", "AttachmentPool.Handle"))

	var wg sync.WaitGroup
	logger.DebugContext(ctx, "processing announcements")
	for i, attachment := range arg.Announcement.Attachments {
		ctx := slogcontext.Append(
			ctx,
			slog.Int("attachment_item", i+1),
			slog.String("filename", attachment.OriginalFilename),
		)
		logger := logger.With()
		if logger.Enabled(ctx, slog.LevelDebug) {
			logger = logger.With(slog.Any("attachment", attachment))
		}
		wg.Go(func() {
			task := AttachmentTask{
				AnnouncementID:    arg.Announcement.ID2,
				AnnouncementTitle: arg.Announcement.AnnouncementTitle,
				StockCode:         arg.Announcement.StockCode,
				TotalAttachment:   len(arg.Announcement.Attachments),
				AttachmentItem:    i + 1,
				Attachment:        attachment,
				Date:              arg.Announcement.Date,
			}
			p.handle(arg.Ctx, task)
			logger.InfoContext(ctx, "processed attachment")
		})
	}
	logger.DebugContext(ctx, "waiting for attachment processing to complete")
	wg.Wait()
	logger.DebugContext(ctx, "processed announcements")
}

func (p *attachmentPool) handle(ctx context.Context, task AttachmentTask) {
	ctx, span := p.tracer.Start(
		ctx,
		"attachmentPool.handle",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := p.logger.With()
	if err := p.processor.Process(ctx, task); err != nil {
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return
	}
}
