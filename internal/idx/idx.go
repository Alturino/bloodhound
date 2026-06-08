package idx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"time"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/semaphore"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/blobstorage"
	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type IDX struct {
	config            *config.Config
	logger            *slog.Logger
	ctx               context.Context
	cancel            context.CancelFunc
	db                *sql.DB
	tracer            trace.Tracer
	metrics           *telemetry.Metrics
	client            Client
	pageCh            chan *Page
	sem               *semaphore.Weighted
	storage           blobstorage.Storage
	announcementStore AnnouncementStore
	attachmentStore   AttachmentStore
}

func NewWorkerIdx(
	ctx context.Context,
	config *config.Config,
	logger *slog.Logger,
	tracer trace.Tracer,
	metrics *telemetry.Metrics,
	announcementStore AnnouncementStore,
	attachmentStore AttachmentStore,
	client Client,
	db *sql.DB,
	storage blobstorage.Storage,
) *IDX {
	ctx, cancel := context.WithCancel(ctx)
	worker := config.App.IDX.WorkerPool.AnnouncementWorkers
	idx := &IDX{
		ctx:               ctx,
		cancel:            cancel,
		config:            config,
		logger:            logger,
		tracer:            tracer,
		metrics:           metrics,
		client:            client,
		pageCh:            make(chan *Page, worker*10),
		sem:               semaphore.NewWeighted(int64(worker)),
		db:                db,
		storage:           storage,
		announcementStore: announcementStore,
		attachmentStore:   attachmentStore,
	}
	idx.Start()
	return idx
}

func (w *IDX) Start() {
	logger := w.logger.With(slog.String("tag", "idx.IDX.Start"))

	if err := w.process(w.ctx); err != nil {
		logger.WarnContext(w.ctx, "seeding", slog.Any("error", err))
	}

	for i := range w.config.App.IDX.WorkerPool.AnnouncementWorkers {
		go w.worker(i)
	}
	w.schedule(w.ctx, w.process)
}

func (w *IDX) schedule(ctx context.Context, onTick func(ctx context.Context) error) {
	interval := w.config.Scheduler.Interval
	logger := w.logger.With(slog.String("tag", "idx.IDX.tick"))
	ticker := time.Tick(interval)
	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "received context done, stopping", slog.Any("error", ctx.Err()))
			return
		case t := <-ticker:
			ctx := slogctx.Append(ctx, slog.Time(constants.ExecutedAt, t))
			logger.DebugContext(ctx, "scheduler executing")
			if err := onTick(ctx); err != nil {
				logger.ErrorContext(ctx, "scheduler executing", slog.Any("error", err))
				continue
			}
			logger.InfoContext(ctx, "scheduler executed")
		}
	}
}

func (w *IDX) process(ctx context.Context) error {
	start := time.Now()
	defer func() {
		w.metrics.IdxProcessingDuration.Record(
			ctx,
			float64(time.Since(start).Milliseconds()),
			metric.WithAttributes(attribute.String(constants.Service, "idx")),
		)
	}()

	ctx, span := w.tracer.Start(
		ctx,
		"idx.IDX.process",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	span.AddEvent("processing idx")

	latestAnnouncement, err := w.announcementStore.LatestAnnouncement(ctx, nil)
	if err != nil {
		latestAnnouncement.Date = time.Time{}
	}
	ctx = slogctx.Append(ctx, slog.Time(constants.LatestAnnouncementDate, latestAnnouncement.Date))

	if err := w.processAnnouncements(ctx, latestAnnouncement.Date); err != nil {
		return err
	}

	span.AddEvent("processed idx")
	return nil
}

func (w *IDX) processAnnouncements(ctx context.Context, since time.Time) error {
	ctx, span := w.tracer.Start(
		ctx, "idx.IDX.processAnnouncements",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	span.AddEvent("processing announcements")

	logger := w.logger.With(slog.String("tag", "idx.IDX.processAnnouncements"))

	resp, err := w.client.FetchAnnouncements(ctx, 0, since)
	if err != nil {
		return err
	}

	totalAnnouncements, pageSize := resp.ResultCount, w.config.App.IDX.PageSize
	pageTotal := totalAnnouncements / pageSize
	ctx = slogctx.Append(
		ctx,
		slog.Int(constants.AnnouncementsTotal, totalAnnouncements),
		slog.Int(constants.PageTotal, pageTotal),
		slog.Int(constants.PageSize, pageSize),
	)

	if pageTotal < 0 {
		logger.InfoContext(ctx, "no pages to process")
		span.AddEvent("no pages to process")
		return nil
	}

	for curr := pageTotal - 1; curr >= 0; curr-- {
		ctx := slogctx.Append(ctx, slog.Int(constants.PageIdx, curr))
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "context done, stop sending page", slog.Any("error", ctx.Err()))
			span.AddEvent("context done, stop sending page")
			return nil
		default:
			logger.DebugContext(ctx, "fetching announcements")
			go func(ctx context.Context, curr, pageTotal int, since time.Time, resp AnnouncementResponse) {
				if err := w.getAndSubmitPage(ctx, curr, pageTotal, since, resp); err != nil {
					return
				}
			}(
				ctx,
				curr,
				pageTotal,
				since,
				resp,
			)
		}
	}

	logger.DebugContext(ctx, "processed announcements")
	span.AddEvent("processed announcements")
	return nil
}

func (w *IDX) getAndSubmitPage(
	ctx context.Context,
	curr, pageTotal int,
	since time.Time,
	resp AnnouncementResponse,
) error {
	ctx, span := w.tracer.Start(
		ctx,
		"idx.IDX.getAndSubmitPage",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	span.AddEvent("fetching announcements")

	logger := w.logger.With(slog.String("tag", "idx.IDX.getAndSubmitPage"))

	logger.DebugContext(ctx, "semaphore acquire")
	span.AddEvent("semaphore acquire")
	if err := w.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer w.sem.Release(1)
	logger.DebugContext(ctx, "semaphore acquired")
	span.AddEvent("semaphore acquired")

	fetchStart := time.Now()
	resp, err := w.client.FetchAnnouncements(ctx, curr, since)
	if err != nil {
		return err
	}
	w.metrics.IdxPageFetchDuration.Record(ctx, float64(time.Since(fetchStart).Milliseconds()))
	w.metrics.IdxPagesFetched.Add(ctx, 1)
	w.metrics.IdxAnnouncementsFetchedTotal.Record(ctx, int64(len(resp.Announcements)))

	logger.DebugContext(ctx, "submitting page")
	span.AddEvent("submitting page")
	page := &Page{
		Ctx:           ctx,
		Index:         curr,
		Total:         pageTotal,
		Params:        resp.SearchParams,
		Announcements: resp.Announcements,
	}
	ctx = slogctx.Append(ctx, slog.Any(constants.Page, page))
	w.pageCh <- page
	logger.InfoContext(ctx, "submitted page")
	span.AddEvent("submitted page")

	return nil
}

func (w *IDX) worker(id int) {
	logger := w.logger.With(slog.String("tag", "idx.IDX.worker"), slog.Int(constants.WorkerID, id))
	for {
		select {
		case <-w.ctx.Done():
			logger.InfoContext(w.ctx, "context done, stopping worker")
			return
		case page, ok := <-w.pageCh:
			ctx := slogctx.Append(
				page.Ctx,
				slog.Int(constants.WorkerID, id),
				slog.Any(constants.Page, page),
			)
			if !ok {
				logger.InfoContext(ctx, "channel closed, stopping worker")
				return
			}
			logger.DebugContext(ctx, "processing page")
			if err := w.processPage(ctx, page.Announcements); err != nil {
				logger.ErrorContext(ctx, "processing page", slog.Any("error", err))
				continue
			}
			logger.InfoContext(ctx, "processed page")
		}
	}
}

func (w *IDX) processPage(ctx context.Context, announcements []Announcement) error {
	ctx, span := w.tracer.Start(ctx, "idx.IDX.processPage")
	defer span.End()

	span.AddEvent("processing page")

	logger := w.logger.With(slog.String("tag", "idx.IDX.processPage"))

	if len(announcements) == 0 {
		return errors.New("no announcements to process")
	}

	logger.DebugContext(ctx, "checking processed announcements")
	span.AddEvent("checking processed announcements")
	idxIDs := make([]string, len(announcements))
	for i, ann := range announcements {
		idxIDs[i] = ann.ID
	}
	processedMap, err := w.announcementStore.IsProcessed(ctx, w.db, idxIDs...)
	if err != nil {
		return err
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		keys := slices.Collect(maps.Keys(processedMap))
		ctx = slogctx.Append(ctx, slog.Any(constants.ProcessedID, slices.Clone(keys[:2])))
	}

	if len(processedMap) == 0 {
		logger.InfoContext(ctx, "no processed announcements")
		span.AddEvent("no processed announcements, saving all")
		if err := w.saveAnnouncements(ctx, announcements); err != nil {
			return err
		}
		span.AddEvent("processed page")
		return nil
	}

	unprocessed := make([]Announcement, 0, len(announcements))
	for _, ann := range announcements {
		if !processedMap[ann.ID] {
			unprocessed = append(unprocessed, ann)
			continue
		}
		w.metrics.IdxAnnouncementsDuplicate.Add(ctx, 1)
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		up := slices.Clone(unprocessed[:2])
		ctx = slogctx.Append(ctx, slog.Any(constants.UnprocessedAnnouncements, up))
	}
	if len(unprocessed) == 0 {
		logger.InfoContext(ctx, "no new announcements")
		span.AddEvent("no new announcements")
		return nil
	}

	logger.DebugContext(ctx, "saving announcements")
	span.AddEvent("saving announcements")
	if err := w.saveAnnouncements(ctx, unprocessed); err != nil {
		err = fmt.Errorf("saving announcements: %v", err)
		return err
	}
	logger.DebugContext(ctx, "saved announcements")
	span.AddEvent("saved announcements")

	span.AddEvent("processed page")
	logger.InfoContext(ctx, "processed page")
	return nil
}

func (w *IDX) saveAnnouncements(ctx context.Context, announcements []Announcement) error {
	ctx, span := w.tracer.Start(
		ctx, "idx.IDX.saveAnnouncements",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	m, _ := baggage.NewMember(constants.Count, strconv.Itoa(len(announcements)))
	bag, _ := baggage.FromContext(ctx).SetMember(m)
	ctx = baggage.ContextWithBaggage(ctx, bag)

	span.AddEvent("saving announcements")

	logger := w.logger.With(
		slog.String("tag", "idx.IDX.saveAnnouncements"),
		slog.Int(constants.AnnouncementsCount, len(announcements)),
	)

	modelAnnouncements := make([]model.Announcements, len(announcements))
	allAttachments := make([]model.Attachments, 0, len(announcements))
	for i, ann := range announcements {
		modelAnnouncements[i] = ann.ToAnnouncement()
		attachments := ann.ToAttachments()
		allAttachments = append(allAttachments, attachments...)
	}
	ctx = slogctx.Append(
		ctx,
		slog.Int(constants.AnnouncementsCount, len(announcements)),
		slog.Int(constants.AttachmentsCount, len(allAttachments)),
	)

	logger.DebugContext(ctx, "inserting announcements")
	span.AddEvent("inserting announcements")
	if err := w.announcementStore.InsertAnnouncement(ctx, w.db, modelAnnouncements...); err != nil {
		return err
	}
	w.metrics.IdxAnnouncementsSaved.Add(ctx, int64(len(announcements)))

	errs := make([]error, 0, len(announcements))
	for attachmentChunks := range slices.Chunk(allAttachments, 1000) {
		if len(attachmentChunks) == 0 {
			break
		}
		if err := w.attachmentStore.InsertAttachment(ctx, w.db, attachmentChunks...); err != nil {
			errs = append(errs, err)
			continue
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}

	logger.InfoContext(ctx, "inserted announcements")
	span.AddEvent("inserted announcements")

	return nil
}

func (w *IDX) Shutdown() {
	w.cancel()
	close(w.pageCh)
}
