package idx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type announcementPool struct {
	logger            *slog.Logger
	metrics           *telemetry.MetricsProvider
	taskChan          chan *Page
	workerCount       int
	cancel            context.CancelFunc
	shutdownOnce      func()
	startOnce         func()
	tracer            trace.Tracer
	ctx               context.Context
	announcementStore AnnouncementStore
	attachmentStore   AttachmentStore
}

func NewAnnouncementPool(
	ctx context.Context,
	workerCount int,
	logger *slog.Logger,
	tracer trace.Tracer,
	metrics *telemetry.MetricsProvider,
	announcementStore AnnouncementStore,
	attachmentStore AttachmentStore,
) *announcementPool {
	ctx, cancel := context.WithCancel(ctx)
	p := &announcementPool{
		logger:            logger,
		tracer:            tracer,
		metrics:           metrics,
		announcementStore: announcementStore,
		attachmentStore:   attachmentStore,
		taskChan:          make(chan *Page, workerCount*10),
		ctx:               ctx,
		cancel:            cancel,
		workerCount:       workerCount,
	}
	p.startOnce = sync.OnceFunc(p.start)
	p.shutdownOnce = sync.OnceFunc(p.shutdown)
	p.Start()
	return p
}

func (p *announcementPool) Start() {
	p.startOnce()
}

func (p *announcementPool) start() {
	for i := range p.workerCount {
		go p.workerLoop(i)
	}
}

func (p *announcementPool) Submit(page *Page) {
	select {
	case <-p.ctx.Done():
		return
	case p.taskChan <- page:
	}
}

func (p *announcementPool) Shutdown() {
	p.shutdownOnce()
}

func (p *announcementPool) shutdown() {
	defer p.cancel()
	close(p.taskChan)
	p.logger.Debug("shutdown announcement pool")
}

func (p *announcementPool) workerLoop(id int) {
	logger := p.logger.With(
		slog.String("tag", "idx.announcementPool.workerLoop"),
		slog.Int(constants.WorkerID, id),
	)
	for {
		select {
		case <-p.ctx.Done():
			logger.DebugContext(p.ctx, "context done, stopping worker")
			return
		case page, ok := <-p.taskChan:
			if !ok {
				logger.WarnContext(p.ctx, "task channel closed, stopping worker")
				return
			}
			ctx := slogctx.Append(
				page.Ctx,
				slog.Int(constants.WorkerID, id),
				slog.Any(constants.Page, page),
			)
			logger.DebugContext(ctx, "processing page")
			if err := p.processPage(ctx, page.Announcements); err != nil {
				logger.ErrorContext(ctx, "processing page", slog.Any("error", err))
				continue
			}
		}
	}
}

func (p *announcementPool) processPage(
	ctx context.Context,
	announcements []Announcement,
) error {
	ctx, span := p.tracer.Start(ctx, "idx.announcementPool.processPage")
	defer span.End()

	logger := p.logger.With(slog.String("tag", "idx.announcementPool.processPage"))

	if len(announcements) == 0 {
		return errors.New("no announcements to process")
	}

	logger.DebugContext(ctx, "checking processed announcements")
	span.AddEvent("checking processed announcements")
	idxIDs := make([]string, len(announcements))
	for i, ann := range announcements {
		idxIDs[i] = ann.ID
	}
	processedMap, err := p.announcementStore.IsProcessed(ctx, nil, idxIDs...)
	if err != nil {
		return err
	}
	if len(processedMap) == 0 {
		logger.DebugContext(ctx, "no processed announcements, saving all")
		span.AddEvent("no processed announcements, saving all")
		if err := p.saveAnnouncements(ctx, announcements); err != nil {
			return fmt.Errorf("saving all announcements: %w", err)
		}
		logger.DebugContext(ctx, "processed announcements, saved all")
		span.AddEvent("processed announcements, saved all")
		return nil
	}
	logger.DebugContext(ctx, "found processed announcements")
	span.AddEvent("found processed announcements")

	logger.DebugContext(ctx, "filtering processed announcements")
	span.AddEvent("filtering processed announcements")
	unprocessed := make([]Announcement, 0, len(announcements))
	for _, ann := range announcements {
		if !processedMap[ann.ID] {
			unprocessed = append(unprocessed, ann)
			continue
		}
	}
	dup := len(announcements) - len(unprocessed)
	ctx = slogctx.Append(ctx, slog.Int("duplicate", dup))
	p.metrics.AnnouncementsSaved.Add(
		ctx,
		int64(dup),
		metric.WithAttributes(
			attribute.String(constants.Status, "duplicate"),
		),
	)
	if len(unprocessed) == 0 {
		logger.DebugContext(ctx, "no new announcements")
		span.AddEvent("no new announcements")
		return nil
	}
	logger.DebugContext(ctx, "filtered processed announcements")
	span.AddEvent("filtered processed announcements")

	logger.DebugContext(ctx, "saving announcements")
	span.AddEvent("saving announcements")
	if err := p.saveAnnouncements(ctx, unprocessed); err != nil {
		err = fmt.Errorf("saving announcements: %w", err)
		return err
	}

	span.AddEvent("processed page")
	logger.InfoContext(ctx, "processed page")
	return nil
}

func (p *announcementPool) saveAnnouncements(
	ctx context.Context,
	announcements []Announcement,
) error {
	ctx, span := p.tracer.Start(ctx, "idx.announcementPool.saveAnnouncements")
	defer span.End()

	logger := p.logger.With(slog.String("tag", "idx.announcementPool.saveAnnouncements"))

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
	if err := p.announcementStore.InsertAnnouncement(ctx, nil, modelAnnouncements...); err != nil {
		return err
	}
	p.metrics.AnnouncementsSaved.Add(ctx, int64(len(announcements)), metric.WithAttributes(
		attribute.String(constants.Status, "saved"),
	))

	logger.DebugContext(ctx, "inserting attachments")
	span.AddEvent("inserting attachments")
	errs := make([]error, 0, len(announcements))
	for attachmentChunks := range slices.Chunk(allAttachments, 1000) {
		if len(attachmentChunks) == 0 {
			break
		}
		if err := p.attachmentStore.InsertAttachment(ctx, nil, attachmentChunks...); err != nil {
			errs = append(errs, err)
			continue
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	logger.InfoContext(ctx, "saved announcements")

	return nil
}
