package idx

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	slogcontext "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
)

type IDX struct {
	config  *config.Config
	logger  *slog.Logger
	ap      AnnouncementPool
	tracer  trace.Tracer
	client  Client
	storage storage.Storage
	store   state.IdxStore
}

func NewWorkerIdx(
	config *config.Config,
	logger *slog.Logger,
	tracer trace.Tracer,
	store state.IdxStore,
	client Client,
	storage storage.Storage,
	announcementPool AnnouncementPool,
) *IDX {
	return &IDX{
		config:  config,
		logger:  logger,
		tracer:  tracer,
		client:  client,
		storage: storage,
		store:   store,
		ap:      announcementPool,
	}
}

func (w IDX) Start(ctx context.Context) error {
	interval := w.config.Scheduler.Interval

	logger := w.logger.With(
		slog.String("tag", "idx.WorkerIdx.Start"),
		slog.Duration("interval", interval),
	)

	if err := w.Process(ctx); err != nil {
		err = fmt.Errorf("initial processing: %w", err)
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return err
	}

	logger.InfoContext(ctx, "started background IDX worker")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "stopping background IDX worker", slog.Any("error", ctx.Err()))
			return ctx.Err()
		case <-ticker.C:
			if err := w.Process(ctx); err != nil {
				return err
			}
		}
	}
}

func (w IDX) Process(ctx context.Context) error {
	ctx, span := w.tracer.Start(
		ctx,
		"idx.WorkerIdx.Process",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := w.logger.With(slog.String("tag", "idx.WorkerIdx.Process"))

	isExists, err := w.store.IsExists(ctx)
	if err != nil {
		err = fmt.Errorf("is announcements exists: %w", err)
		return err
	}

	if !isExists {
		logger.InfoContext(ctx, "no existing announcements found, starting seeding")
		span.AddEvent("no existing announcements found, starting seeding")
		return w.processInitial(ctx)
	}

	logger.InfoContext(ctx, "announcements exists starting incremental sync")
	span.AddEvent("announcements exists starting incremental sync")
	return w.processIncremental(ctx)
}

func (w IDX) processInitial(ctx context.Context) error {
	return w.processAnnouncements(ctx, time.Time{}, "processInitial")
}

func (w IDX) processIncremental(ctx context.Context) error {
	latestAnnouncement, err := w.store.LatestAnnouncement(ctx)
	if err != nil {
		return err
	}
	return w.processAnnouncements(ctx, latestAnnouncement.Date, "processIncremental")
}

func (w IDX) processAnnouncements(ctx context.Context, since time.Time, tag string) error {
	ctx, span := w.tracer.Start(ctx, "idx.WorkerIdx."+tag)
	defer span.End()

	ctx = slogcontext.Append(ctx, slog.String("tag", "idx.WorkerIdx."+tag))
	logger := w.logger.With()

	resp, err := w.client.FetchAnnouncements(ctx, 0, since)
	if err != nil {
		err = fmt.Errorf("get total items and pages: %w", err)
		logger.ErrorContext(ctx, err.Error(), slog.Any("error", err))
		return err
	}
	totalItems, pageSize := resp.ResultCount, w.config.App.IDX.PageSize
	totalPages := totalItems / pageSize

	workerCount := w.config.App.IDX.WorkerPool.AnnouncementWorkers
	ctx = slogcontext.Append(ctx,
		slog.Int("total_items", totalItems),
		slog.Int("page_size", pageSize),
		slog.Int("total_pages", totalPages),
		slog.Int("worker_count", workerCount),
	)
	span.SetAttributes(
		attribute.Int("total_items", totalItems),
		attribute.Int("page_size", pageSize),
		attribute.Int("total_pages", totalPages),
		attribute.Int("worker_count", workerCount),
	)

	for page, processedPage := totalPages-1, 0; page >= 0; page, processedPage = page-1, processedPage+1 {
		ctx := slogcontext.Append(
			ctx,
			slog.Int("page", page),
			slog.Int("processed_page", processedPage),
		)

		resp, err := w.client.FetchAnnouncements(ctx, page, time.Time{})
		if err != nil {
			continue
		}
		if resp.ResultCount == 0 || len(resp.Announcements) == 0 {
			logger.InfoContext(ctx, "page empty, stopping")
			break
		}

		if logger.Enabled(ctx, slog.LevelDebug) {
			ctx = slogcontext.Append(
				ctx,
				slog.Int("announcements_count", len(resp.Announcements)),
				slog.Any("announcements", resp.Announcements),
			)
		}
		logger.DebugContext(ctx, "fetched announcements")

		w.ap.Process(ctx, page, resp.Announcements)
		logger.InfoContext(ctx, "processed page")
	}

	return nil
}
