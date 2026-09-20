package idx

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/semaphore"

	"github.com/alturino/bloodhound/internal/config"
	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type AnnouncementScheduler struct {
	config            *config.Scheduler
	logger            *slog.Logger
	startOnce         func()
	shutdownOnce      func()
	cancel            context.CancelFunc
	metrics           *telemetry.MetricsProvider
	pageSize          int
	pool              *announcementPool
	ticker            *time.Ticker
	sem               *semaphore.Weighted
	ctx               context.Context
	tracer            trace.Tracer
	client            Client
	announcementStore AnnouncementStore
	attachmentStore   AttachmentStore
}

func NewAnnouncementScheduler(
	ctx context.Context,
	cfg *config.Scheduler,
	pageSize int,
	workerCount int,
	logger *slog.Logger,
	tracer trace.Tracer,
	metrics *telemetry.MetricsProvider,
	announcementStore AnnouncementStore,
	attachmentStore AttachmentStore,
	client Client,
	pool *announcementPool,
) *AnnouncementScheduler {
	ctx, cancel := context.WithCancel(ctx)

	as := &AnnouncementScheduler{
		ctx:               ctx,
		cancel:            cancel,
		config:            cfg,
		logger:            logger,
		tracer:            tracer,
		metrics:           metrics,
		client:            client,
		sem:               semaphore.NewWeighted(int64(workerCount)),
		pageSize:          pageSize,
		announcementStore: announcementStore,
		attachmentStore:   attachmentStore,
		pool:              pool,
		ticker:            time.NewTicker(cfg.Interval),
	}
	as.startOnce = sync.OnceFunc(as.start)
	as.shutdownOnce = sync.OnceFunc(as.shutdown)
	return as
}

func (s *AnnouncementScheduler) Start() {
	s.startOnce()
}

func (s *AnnouncementScheduler) start() {
	logger := s.logger.With(slog.String("tag", "idx.AnnouncementScheduler.Start"))

	if err := s.process(s.ctx); err != nil {
		logger.WarnContext(s.ctx, "seeding", slog.Any("error", err))
	}

	logger.DebugContext(s.ctx, "starting")
	s.pool.Start()
	go s.schedule()
	logger.InfoContext(s.ctx, "started")
}

func (s *AnnouncementScheduler) Shutdown() {
	s.shutdownOnce()
}

func (s *AnnouncementScheduler) shutdown() {
	defer s.ticker.Stop()
	defer s.cancel()
	s.pool.Shutdown()
	s.logger.Info("shutdown announcement scheduler")
}

func (s *AnnouncementScheduler) schedule() {
	interval := s.config.Interval
	logger := s.logger.With(
		slog.String("tag", "idx.AnnouncementScheduler.schedule"),
		slog.Duration(constants.Interval, interval),
	)

	for {
		select {
		case <-s.ctx.Done():
			logger.InfoContext(s.ctx, "context done, stopping", slog.Any("error", s.ctx.Err()))
			return
		case t := <-s.ticker.C:
			ctx := slogctx.Append(s.ctx, slog.Time(constants.ExecutedAt, t))
			logger.DebugContext(ctx, "scheduler executing")
			if err := s.process(ctx); err != nil {
				logger.ErrorContext(ctx, "scheduler executing", slog.Any("error", err))
				continue
			}
			logger.InfoContext(ctx, "scheduler executed")
		}
	}
}

func (s *AnnouncementScheduler) process(ctx context.Context) error {
	start := time.Now()
	defer func() {
		s.metrics.IdxProcessingDuration.Record(
			ctx,
			float64(time.Since(start).Milliseconds()),
			metric.WithAttributes(attribute.String(constants.Service, "idx")),
		)
	}()

	ctx, span := s.tracer.Start(
		ctx, "idx.AnnouncementScheduler.process",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "idx.AnnouncementScheduler.process"))

	logger.DebugContext(ctx, "get latest announcement")
	span.AddEvent("get latest announcement")
	latestAnnouncement, err := s.announcementStore.LatestAnnouncement(ctx, nil)
	if err != nil {
		logger.DebugContext(ctx, "no latest announcement, set to zero")
		latestAnnouncement.Date = time.Time{}
	}
	ctx = slogctx.Append(ctx, slog.Time(constants.LatestAnnouncementDate, latestAnnouncement.Date))
	logger.DebugContext(ctx, "got latest announcement")
	span.AddEvent("got latest announcement")

	logger.DebugContext(ctx, "processing announcements")
	span.AddEvent("processing announcements")
	if err := s.processAnnouncements(ctx, latestAnnouncement.Date); err != nil {
		err = fmt.Errorf("processing announcements: %w", err)
		return err
	}
	logger.InfoContext(ctx, "processed announcements")
	span.AddEvent("processed announcements")

	return nil
}

func (s *AnnouncementScheduler) processAnnouncements(ctx context.Context, since time.Time) error {
	ctx, span := s.tracer.Start(
		ctx, "idx.AnnouncementScheduler.processAnnouncements",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "idx.AnnouncementScheduler.processAnnouncements"))

	resp, err := s.client.FetchAnnouncements(ctx, 0, since)
	if err != nil {
		return err
	}

	totalAnnouncements, pageSize := resp.ResultCount, s.pageSize
	pageTotal := totalAnnouncements / pageSize
	ctx = slogctx.Append(
		ctx,
		slog.Int(constants.AnnouncementsTotal, totalAnnouncements),
		slog.Int(constants.PageTotal, pageTotal),
		slog.Int(constants.PageSize, pageSize),
	)

	var wg sync.WaitGroup
	for curr := pageTotal; curr >= 0; curr-- {
		ctx := slogctx.Append(ctx, slog.Int(constants.PageIdx, curr))
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "context done, stop sending page")
			span.AddEvent("context done, stop sending page")
			wg.Wait()
			return nil
		default:
			logger.DebugContext(ctx, "get and submit announcements")
			span.AddEvent("get and submit announcements")
			wg.Go(func() {
				logger.DebugContext(ctx, "working on page")
				if err := s.getAndSubmitPage(ctx, curr, pageTotal, since, resp); err != nil {
					logger.ErrorContext(ctx, "submitting page", slog.Any("error", err))
					return
				}
				logger.DebugContext(ctx, "worked on page")
			})
		}
	}
	wg.Wait()
	logger.DebugContext(ctx, "finished waiting")

	return nil
}

func (s *AnnouncementScheduler) getAndSubmitPage(
	ctx context.Context,
	curr, pageTotal int,
	since time.Time,
	resp AnnouncementResponse,
) error {
	ctx, span := s.tracer.Start(
		ctx, "idx.AnnouncementScheduler.getAndSubmitPage",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.Int(constants.PageIdx, curr),
			attribute.Int(constants.PageTotal, pageTotal),
		),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "idx.AnnouncementScheduler.getAndSubmitPage"))

	logger.DebugContext(ctx, "acquiring semaphore")
	span.AddEvent("acquiring semaphore")
	if err := s.sem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("acquiring semaphore: %w", err)
	}
	defer s.sem.Release(1)
	logger.DebugContext(ctx, "semaphore acquired")
	span.AddEvent("semaphore acquired")

	fetchStart := time.Now()
	resp, err := s.client.FetchAnnouncements(ctx, curr, since)
	if err != nil {
		err = fmt.Errorf("get page: %w", err)
		return err
	}
	s.metrics.IdxPageFetchDuration.Record(ctx, float64(time.Since(fetchStart).Milliseconds()))
	s.metrics.IdxPagesFetched.Add(ctx, 1)
	s.metrics.IdxAnnouncementsFetchedTotal.Record(ctx, int64(len(resp.Announcements)))

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
	s.pool.Submit(page)
	logger.DebugContext(ctx, "submitted page")
	span.AddEvent("submitted page")

	return nil
}
