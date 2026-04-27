package idx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	slogcontext "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/state"
)

type AnnouncementProcessor interface {
	ProcessAnnouncement(ctx context.Context, ann models.Announcement) error
}

type announcement struct {
	logger         *slog.Logger
	tracer         trace.Tracer
	client         Client
	store          state.IdxStore
	attachmentPool AttachmentPool
}

func NewAnnouncementProcessor(
	logger *slog.Logger,
	tracer trace.Tracer,
	client Client,
	store state.IdxStore,
	attachmentPool AttachmentPool,
) AnnouncementProcessor {
	return announcement{
		logger:         logger,
		tracer:         tracer,
		client:         client,
		store:          store,
		attachmentPool: attachmentPool,
	}
}

func (a announcement) ProcessAnnouncement(ctx context.Context, ann models.Announcement) error {
	ctx, span := a.tracer.Start(
		ctx,
		"worker.AnnouncementProcessor.processAnnouncement",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("tag", "worker.WorkerIdx.processAnnouncement"),
			attribute.String("idx_announcement_id", ann.ID2),
			attribute.String("stock_code", ann.StockCode),
			attribute.String("announcement_date", ann.Date.String()),
		),
	)
	defer span.End()

	ctx = slogcontext.With(ctx,
		slog.String("tag", "worker.WorkerIdx.processAnnouncement"),
		slog.String("idx_announcement_id", ann.ID2),
		slog.String("stock_code", ann.StockCode),
		slog.Time("announcement_date", ann.Date),
	)

	if err := a.store.RecordAnnouncement(ctx, ann); err != nil {
		err = fmt.Errorf("record announcement: %w", err)
		return err
	}

	attachmentResult, err := a.attachmentPool.Handle(ctx, HandleAttachmentsArgs{
		Announcement: ann,
	})
	if err != nil {
		return err
	}

	for _, attachment := range attachmentResult {
		err = errors.Join(err, attachment.Err)
	}

	return err
}
