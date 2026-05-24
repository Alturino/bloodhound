package idx

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"time"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/semaphore"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/blobstorage"
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
	pageCh            chan Page
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
		pageCh:            make(chan Page, worker*10),
		sem:               semaphore.NewWeighted(int64(worker)),
		db:                db,
		storage:           storage,
		announcementStore: announcementStore,
		attachmentStore:   attachmentStore,
	}
	return idx
}

func (w *IDX) Start() {
	logger := w.logger.With(slog.String("tag", "idx.IDX.Start"))

	logger.DebugContext(w.ctx, "seeding")
	if err := w.process(w.ctx); err != nil {
		logger.WarnContext(w.ctx, "seeding", slog.Any("error", err))
	}
	logger.DebugContext(w.ctx, "done seeding")

	logger.DebugContext(w.ctx, "started scheduler work")
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
			ctx := slogctx.Append(ctx, slog.Time("executed_at", t))
			logger.DebugContext(ctx, "executing")
			if err := onTick(ctx); err != nil {
				logger.ErrorContext(ctx, "scheduler executing", slog.Any("error", err))
				continue
			}
			logger.InfoContext(ctx, "executed")
		}
	}
}

func (w *IDX) process(ctx context.Context) error {
	start := time.Now()
	defer func() {
		w.metrics.IdxProcessingDuration.Record(
			ctx,
			float64(time.Since(start).Milliseconds()),
			metric.WithAttributes(attribute.String("service", "idx")),
		)
	}()

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

	if pageTotal < 0 {
		logger.InfoContext(ctx, "no pages to process")
		span.AddEvent("no pages to process")
		return nil
	}

	for curr := pageTotal - 1; curr >= 0; curr-- {
		ctx := slogctx.Append(ctx, slog.Int("page_idx", curr))
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "context done, stop sending page", slog.Any("error", ctx.Err()))
			return nil
		default:
			logger.DebugContext(ctx, "fetching announcements")
			span.AddEvent("fetching announcements")
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

	logger := w.logger.With(slog.String("tag", "idx.IDX.getAndSubmitPage"))

	if err := w.sem.Acquire(ctx, 1); err != nil {
		logger.ErrorContext(ctx, "acquire semaphore", slog.Any("error", err))
		return err
	}
	defer w.sem.Release(1)

	fetchStart := time.Now()
	resp, err := w.client.FetchAnnouncements(ctx, curr, since)
	if err != nil {
		return err
	}
	w.metrics.IdxPageFetchDuration.Record(ctx, float64(time.Since(fetchStart).Milliseconds()))
	w.metrics.IdxPagesFetched.Add(ctx, 1)
	w.metrics.IdxAnnouncementsFetchedTotal.Record(ctx, int64(len(resp.Announcements)))
	page := Page{
		Ctx:           ctx,
		Index:         curr,
		Total:         pageTotal,
		Params:        resp.SearchParams,
		Announcements: resp.Announcements,
	}
	ctx = slogctx.Append(ctx, slog.Any("page", page))
	logger.InfoContext(ctx, "fetched announcements")

	logger.DebugContext(ctx, "sending page")
	w.pageCh <- page
	logger.InfoContext(ctx, "sent page")
	return nil
}

func (w *IDX) worker(id int) {
	logger := w.logger.With(slog.String("tag", "idx.IDX.worker"), slog.Int("worker_id", id))
	for {
		select {
		case <-w.ctx.Done():
			logger.InfoContext(w.ctx, "context done, stopping worker")
			return
		case page, ok := <-w.pageCh:
			ctx := slogctx.Append(page.Ctx, slog.Int("worker_id", id), slog.Any("page", page))
			logger.DebugContext(ctx, "received page")
			if !ok {
				logger.InfoContext(ctx, "channel closed, stopping worker")
				return
			}
			logger.InfoContext(ctx, "processing page")
			if err := w.processPage(ctx, page.Announcements); err != nil {
				logger.ErrorContext(ctx, "processing page", slog.Any("error", err))
				continue
			}
			logger.InfoContext(ctx, "processed page")
		}
	}
}

func (w *IDX) processPage(ctx context.Context, announcements []Announcement) error {
	ctx, span := w.tracer.Start(ctx, "idx.IDX.processAnnouncementPage")
	defer span.End()

	logger := w.logger.With(slog.String("tag", "idx.IDX.processAnnouncementPage"))

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
		if len(keys) >= 2 {
			ctx = slogctx.Append(ctx, slog.Any("processed_id", slices.Clone(keys[:2])))
		}
	}

	if len(processedMap) == 0 {
		logger.InfoContext(ctx, "no processed announcements")
		if err := w.saveAnnouncements(ctx, announcements); err != nil {
			return err
		}
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
		up := slices.Clone(unprocessed[:5])
		ctx = slogctx.Append(ctx, slog.Any("unprocessed_announcements", up))
	}
	if len(unprocessed) == 0 {
		logger.InfoContext(ctx, "no new announcements")
		return nil
	}

	logger.DebugContext(ctx, "saving announcements")
	span.AddEvent("saving announcements")
	if err := w.saveAnnouncements(ctx, unprocessed); err != nil {
		return err
	}

	return nil
}

func (w *IDX) saveAnnouncements(ctx context.Context, announcements []Announcement) error {
	ctx, span := w.tracer.Start(
		ctx, "idx.IDX.SaveAnnouncements",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.Int("count", len(announcements))),
	)
	defer span.End()

	logger := w.logger.With(
		slog.String("tag", "idx.IDX.saveAnnouncements"),
		slog.Int("announcements_count", len(announcements)),
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
		slog.Int("announcements_count", len(announcements)),
		slog.Int("attachments_count", len(allAttachments)),
	)

	logger.DebugContext(ctx, "inserting announcements")
	span.AddEvent("inserting announcements")
	if err := w.announcementStore.InsertAnnouncement(ctx, w.db, modelAnnouncements...); err != nil {
		return err
	}
	w.metrics.IdxAnnouncementsSaved.Add(ctx, int64(len(announcements)))
	logger.DebugContext(ctx, "inserted announcements")
	span.AddEvent("inserted announcements")

	logger.DebugContext(ctx, "inserted attachments")
	span.AddEvent("inserted attachments")
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
	logger.DebugContext(ctx, "inserted attachments")
	span.AddEvent("inserted attachments")

	logger.InfoContext(ctx, "inserted announcements")

	return errors.Join(errs...)
}

func (w *IDX) Shutdown() {
	w.cancel()
	close(w.pageCh)
}
