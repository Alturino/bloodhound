package idx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-jet/jet/v2/qrm"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/blobstorage"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type IDX struct {
	config            *config.Config
	logger            *slog.Logger
	ctx               context.Context
	db                *sql.DB
	tracer            trace.Tracer
	client            Client
	storage           blobstorage.Storage
	announcementStore AnnouncementStore
	attachmentStore   AttachmentStore
}

func NewWorkerIdx(
	ctx context.Context,
	config *config.Config,
	logger *slog.Logger,
	tracer trace.Tracer,
	announcementStore AnnouncementStore,
	attachmentStore AttachmentStore,
	client Client,
	db *sql.DB,
	storage blobstorage.Storage,
) *IDX {
	return &IDX{
		ctx:               ctx,
		config:            config,
		logger:            logger,
		tracer:            tracer,
		client:            client,
		db:                db,
		storage:           storage,
		announcementStore: announcementStore,
		attachmentStore:   attachmentStore,
	}
}

func (w *IDX) Start() {
	logger := w.logger.With(slog.String("tag", "idx.IDX.Start"))

	logger.DebugContext(w.ctx, "seeding")
	if err := w.Process(w.ctx); err != nil {
		logger.WarnContext(w.ctx, "seeding", slog.Any("error", err))
	}
	logger.DebugContext(w.ctx, "done seeding")

	tick := time.Tick(w.config.Scheduler.Interval)
	logger.DebugContext(w.ctx, "started scheduler work")
	for {
		select {
		case <-w.ctx.Done():
			err := w.ctx.Err()
			logger.InfoContext(w.ctx, "received context done, stopping", slog.Any("error", err))
			return
		case t := <-tick:
			logger.InfoContext(w.ctx, "executing", slog.Time("executed_at", t))
			w.looper(w.ctx, w.Process)
		}
	}
}

func (w *IDX) looper(ctx context.Context, run func(ctx context.Context) error) {
	interval := w.config.Scheduler.Interval
	logger := w.logger.With(
		slog.String("tag", "idx.IDX.looper"),
		slog.Duration("interval", interval),
	)
	ticker := time.Tick(interval)
	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "received context done, stopping", slog.Any("error", ctx.Err()))
			return
		case t := <-ticker:
			logger := logger.With(slog.Time("executed_at", t))
			logger.DebugContext(ctx, "excuting")
			if err := run(ctx); err != nil {
				logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
				continue
			}
			logger.InfoContext(ctx, "executed")
		}
	}
}

func (w *IDX) Process(ctx context.Context) error {
	ctx, span := w.tracer.Start(
		ctx,
		"idx.IDX.Process",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	latestAnnouncement, err := w.announcementStore.LatestAnnouncement(ctx, nil)
	if err != nil {
		latestAnnouncement.Date = time.Time{}
	}
	ctx = slogctx.Append(ctx, slog.Time("latest_announcement_date", latestAnnouncement.Date))

	return w.processAnnouncements(ctx, latestAnnouncement.Date)
}

func (w *IDX) processAnnouncements(ctx context.Context, since time.Time) error {
	ctx, span := w.tracer.Start(
		ctx, "idx.IDX.processAnnouncements",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String("latest_announcement_date", since.String())),
	)
	defer span.End()

	logger := w.logger.With(slog.String("tag", "idx.IDX.processAnnouncements"))

	logger.DebugContext(ctx, "initial fetching announcements")
	span.AddEvent("initial fetching announcements")
	resp, err := w.client.FetchAnnouncements(ctx, 0, since)
	if err != nil {
		return err
	}

	totalAnnouncements, pageSize := resp.ResultCount, w.config.App.IDX.PageSize
	pageTotal := totalAnnouncements / pageSize
	ctx = slogctx.Append(
		ctx,
		slog.Int("announcements_total", totalAnnouncements),
		slog.Int("page_total", pageTotal),
		slog.Int("page_size", pageSize),
	)
	span.SetAttributes(
		attribute.Int("announcements_total", totalAnnouncements),
		attribute.Int("page_total", pageTotal),
		attribute.Int("page_size", pageSize),
	)

	for page, curr := pageTotal, 1; page >= 0; page, curr = page-1, curr+1 {
		ctx := slogctx.Append(ctx, slog.Int("page", page), slog.Int("processed_page", curr))

		logger.DebugContext(ctx, "fetching announcements")
		span.AddEvent("fetching announcements")
		resp, err := w.client.FetchAnnouncements(ctx, page, since)
		if err != nil {
			logger.ErrorContext(ctx, "fetch announcements", slog.Any("error", err))
			continue
		}
		if resp.ResultCount == 0 || len(resp.Announcements) == 0 {
			logger.InfoContext(ctx, "page empty, stopping")
			break
		}
		logger.DebugContext(ctx, "fetched announcements")
		span.AddEvent("fetched announcements")

		logger.DebugContext(ctx, "processing announcements page")
		span.AddEvent("processing announcements page")
		if err := w.processAnnouncementPage(ctx, resp.Announcements); err != nil {
			logger.ErrorContext(ctx, "processing announcements page", slog.Any("error", err))
			continue
		}
		logger.InfoContext(ctx, "processed announcements page")
		span.AddEvent("processed announcements page")
	}

	return nil
}

func (w *IDX) processAnnouncementPage(
	ctx context.Context,
	announcements []Announcement,
) error {
	ctx, span := w.tracer.Start(ctx, "idx.IDX.processAnnouncementPage")
	defer span.End()

	logger := w.logger.With(slog.String("tag", "idx.IDX.processAnnouncementPage"))

	logger.DebugContext(ctx, "begin transaction")
	span.AddEvent("begin transaction")
	tx, err := w.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		err := fmt.Errorf("begin transaction: %w", err)
		logger.ErrorContext(ctx, "begin transaction", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			if !errors.Is(err, sql.ErrTxDone) {
				logger.ErrorContext(ctx, "rollback transaction", slog.Any("error", err))
				telemetry.RecordError(span, err)
				return
			}
			logger.InfoContext(ctx, "rollback on closed tx")
		}
	}()

	logger.DebugContext(ctx, "checking processed announcements")
	span.AddEvent("checking processed announcements")
	idxIDs := make([]string, len(announcements))
	for i, ann := range announcements {
		idxIDs[i] = ann.ID
	}
	processedMap, err := w.announcementStore.IsProcessed(ctx, tx, idxIDs...)
	if err != nil {
		logger.ErrorContext(ctx, "batch check processed", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return err
	}
	if len(processedMap) == 0 {
		logger.InfoContext(ctx, "no processed announcements")
		if err := w.SaveAnnouncements(ctx, tx, announcements); err != nil {
			logger.ErrorContext(ctx, "save announcements", slog.Any("error", err))
			telemetry.RecordError(span, err)
			return err
		}
	}

	unprocessed := make([]Announcement, 0, len(announcements))
	for _, ann := range announcements {
		if !processedMap[ann.ID] {
			unprocessed = append(unprocessed, ann)
			continue
		}
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		if len(unprocessed) <= 10 {
			ctx = slogctx.Append(ctx, slog.Any("unprocessed_announcements", unprocessed))
		}
	}
	if len(unprocessed) == 0 {
		logger.InfoContext(ctx, "no new announcements")
		return nil
	}

	logger.DebugContext(ctx, "saving announcements")
	span.AddEvent("saving announcements")
	if err := w.SaveAnnouncements(ctx, tx, unprocessed); err != nil {
		logger.ErrorContext(ctx, "saving announcements", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return err
	}

	logger.DebugContext(ctx, "committing transaction")
	span.AddEvent("committing transaction")
	if err := tx.Commit(); err != nil {
		logger.ErrorContext(ctx, "committing transaction", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return err
	}
	logger.DebugContext(ctx, "committed transaction")
	span.AddEvent("committed transaction")

	return nil
}

func (w *IDX) SaveAnnouncements(
	ctx context.Context,
	tx qrm.DB,
	announcements []Announcement,
) error {
	ctx, span := w.tracer.Start(
		ctx, "idx.IDX.SaveAnnouncements",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.Int("count", len(announcements))),
	)
	defer span.End()

	logger := w.logger.With(
		slog.String("tag", "idx.IDX.SaveAnnouncements"),
		slog.Int("announcements_count", len(announcements)),
	)

	modelAnnouncements := make([]model.Announcements, len(announcements))
	allAttachments := make([]model.Attachments, 0, len(announcements))
	for i, ann := range announcements {
		modelAnnouncements[i] = ann.ToAnnouncement()
		attachments := ann.ToAttachments()
		allAttachments = append(allAttachments, attachments...)
	}

	logger.DebugContext(ctx, "inserting announcements")
	span.AddEvent("inserting announcements")
	if err := w.announcementStore.InsertAnnouncement(ctx, tx, modelAnnouncements...); err != nil {
		logger.ErrorContext(ctx, "inserting announcements", slog.Any("error", err))
		return err
	}
	logger.DebugContext(ctx, "inserted announcements")
	span.AddEvent("inserted announcements")

	logger.DebugContext(ctx, "inserted attachments")
	span.AddEvent("inserted attachments")
	if err := w.attachmentStore.InsertAttachment(ctx, tx, allAttachments...); err != nil {
		logger.ErrorContext(ctx, "inserting attachments", slog.Any("error", err))
		return err
	}
	logger.DebugContext(ctx, "inserted attachments")
	span.AddEvent("inserted attachments")

	logger.InfoContext(
		ctx,
		"announcements and attachments",
		slog.Int("announcements", len(announcements)),
		slog.Int("attachments", len(allAttachments)),
	)

	return nil
}
