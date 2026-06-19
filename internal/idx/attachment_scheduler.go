package idx

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// AttachmentScheduler polls the store for unprocessed attachments and submits
// them to an internal worker pool on a configurable interval.
//
// It implements the Background interface (Start, Shutdown).
type AttachmentScheduler struct {
	config  *config.Scheduler
	logger  *slog.Logger
	tracer  trace.Tracer
	metrics *telemetry.Metrics
	store   AttachmentStore
	pool    *attachmentPool
	ctx     context.Context
	cancel  context.CancelFunc
	ticker  *time.Ticker
}

// NewAttachmentScheduler creates a new AttachmentScheduler with the given
// worker pool as a dependency. The pool is not started until Start() is called.
func NewAttachmentScheduler(
	ctx context.Context,
	cfg *config.Scheduler,
	logger *slog.Logger,
	tracer trace.Tracer,
	metrics *telemetry.Metrics,
	store AttachmentStore,
	pool *attachmentPool,
) *AttachmentScheduler {
	ctx, cancel := context.WithCancel(ctx)

	scheduler := &AttachmentScheduler{
		config:  cfg,
		logger:  logger,
		tracer:  tracer,
		metrics: metrics,
		store:   store,
		pool:    pool,
		ctx:     ctx,
		cancel:  cancel,
	}
	scheduler.Start()
	return scheduler
}

// Start starts the internal worker pool and launches the schedule goroutine
// that polls for unprocessed attachments on a configurable interval.
func (s *AttachmentScheduler) Start() {
	go s.schedule()
}

func (s *AttachmentScheduler) schedule() {
	interval := s.config.Interval
	logger := s.logger.With(
		slog.String("tag", "idx.attachmentScheduler.schedule"),
		slog.Duration(constants.Interval, interval),
	)

	logger.InfoContext(s.ctx, "starting scheduler", slog.Duration(constants.Interval, interval))
	s.ticker = time.NewTicker(interval)
	defer s.ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			logger.InfoContext(s.ctx, "context done, stopping", slog.Any("error", s.ctx.Err()))
			return
		case t := <-s.ticker.C:
			ctx := slogctx.Append(s.ctx, slog.Time(constants.ExecutedAt, t))
			logger.DebugContext(ctx, "scheduler executing")
			s.pollAndSubmit(ctx)
		}
	}
}

func (s *AttachmentScheduler) pollAndSubmit(ctx context.Context) {
	ctx, span := s.tracer.Start(
		ctx,
		"idx.attachmentScheduler.pollAndSubmit",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	span.AddEvent("polling and submitting")

	logger := s.logger.With(slog.String("tag", "idx.attachmentScheduler.pollAndSubmit"))

	attachments, err := s.store.UnprocessedAttachments(ctx)
	if err != nil {
		telemetry.RecordError(span, err)
		logger.ErrorContext(ctx, "failed to get unprocessed attachments", slog.Any("error", err))
		return
	}

	s.metrics.AttPolledCount.Record(ctx, int64(len(attachments)))

	if len(attachments) == 0 {
		logger.InfoContext(ctx, "no unprocessed attachments")
		span.AddEvent("no unprocessed attachments")
		return
	}

	ctx = slogctx.Append(ctx, slog.Int(constants.UnprocessedAttachmentsCount, len(attachments)))

	ids := make([]uuid.UUID, len(attachments))
	for i, att := range attachments {
		ids[i] = att.ID
	}

	claimed, err := s.store.ClaimAttachments(ctx, ids...)
	if err != nil {
		telemetry.RecordError(span, err)
		logger.ErrorContext(ctx, "failed to claim attachments", slog.Any("error", err))
		return
	}

	for i := range claimed {
		select {
		case <-s.ctx.Done():
			logger.InfoContext(ctx, "context done, stopping submission")
			span.AddEvent("context done, stopping submission")
			return
		default:
			s.pool.Submit(&AttachmentTask{Ctx: ctx, Attachment: claimed[i]})
		}
	}

	logger.InfoContext(ctx, "submitted attachments", slog.Int(constants.Count, len(claimed)))
	span.AddEvent("submitted attachments")
}

// Shutdown cancels the scheduler context and shuts down the internal worker pool.
func (s *AttachmentScheduler) Shutdown() {
	s.cancel()
	s.pool.Shutdown()
	s.logger.Info("shutdown attachment scheduler")
}
