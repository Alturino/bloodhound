package idx

import (
	"context"
	"log/slog"

	slogcontext "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/state"
)

type AnnouncementProcessor interface {
	Process(ctx context.Context, args *AnnouncementTask) error
}

type announcement struct {
	logger         *slog.Logger
	tracer         trace.Tracer
	client         Client
	store          state.IdxStore
	attachmentPool Attachment
}

func NewAnnouncementProcessor(
	logger *slog.Logger,
	tracer trace.Tracer,
	client Client,
	store state.IdxStore,
	attachmentPool Attachment,
) AnnouncementProcessor {
	return announcement{
		logger:         logger,
		tracer:         tracer,
		client:         client,
		store:          store,
		attachmentPool: attachmentPool,
	}
}

func (a announcement) Process(ctx context.Context, args *AnnouncementTask) error {
	ctx, span := a.tracer.Start(
		ctx,
		"idx.AnnouncementProcessor.processAnnouncement",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("tag", "idx.announcement.Process"),
			attribute.String("announcement_id", args.Announcement.ID2),
			attribute.String("announcement_id", args.Announcement.AnnouncementTitle),
			attribute.String("announcement_date", args.Announcement.Date.String()),
			attribute.String("stock_code", args.Announcement.StockCode),
		),
	)
	defer span.End()

	ctx = slogcontext.Append(ctx,
		slog.String("tag", "idx.announcement.Process"),
		slog.String("announcement_id", args.Announcement.ID2),
		slog.String("announcement_title", args.Announcement.AnnouncementTitle),
		slog.Time("announcement_date", args.Announcement.Date),
		slog.String("stock_code", args.Announcement.StockCode),
	)

	if err := a.store.RecordAnnouncement(ctx, args.Announcement); err != nil {
		return err
	}

	a.attachmentPool.Handle(ctx, args)

	return nil
}
