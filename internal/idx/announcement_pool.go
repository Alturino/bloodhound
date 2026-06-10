package idx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type announcementPool struct {
	logger            *slog.Logger
	tracer            trace.Tracer
	metrics           *telemetry.Metrics
	taskChan          chan *Page
	workerCount       int
	ctx               context.Context
	cancel            context.CancelFunc
	announcementStore AnnouncementStore
	attachmentStore   AttachmentStore
}

func NewAnnouncementPool(
	workerCount int,
	logger *slog.Logger,
	tracer trace.Tracer,
	metrics *telemetry.Metrics,
	announcementStore AnnouncementStore,
	attachmentStore AttachmentStore,
) *announcementPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &announcementPool{
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
}

func (p *announcementPool) Start() {
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
	p.cancel()
	close(p.taskChan)
	p.logger.Info("shutdown announcement pool")
}

func (p *announcementPool) workerLoop(id int) {
	logger := p.logger.With(
		slog.String("tag", "idx.announcementPool.workerLoop"),
		slog.Int(constants.WorkerID, id),
	)
	for {
		select {
		case <-p.ctx.Done():
			logger.InfoContext(p.ctx, "context done, stopping worker")
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
			logger.InfoContext(ctx, "processed page")
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
		return fmt.Errorf("checking processed announcements: %w", err)
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		keys := make([]string, 0, len(processedMap))
		for k := range processedMap {
			keys = append(keys, k)
		}
		ctx = slogctx.Append(ctx, slog.Any(constants.ProcessedID, keys[:min(2, len(keys))]))
	}
	if len(processedMap) == 0 {
		logger.InfoContext(ctx, "no processed announcements")
		span.AddEvent("no processed announcements, saving all")
		if err := p.saveAnnouncements(ctx, announcements); err != nil {
			return fmt.Errorf("saving all announcements: %w", err)
		}
		span.AddEvent("processed page")
		return nil
	}

	logger.DebugContext(ctx, "found processed announcements")
	span.AddEvent("found processed announcements")

	unprocessed := make([]Announcement, 0, len(announcements))
	for _, ann := range announcements {
		if !processedMap[ann.ID] {
			unprocessed = append(unprocessed, ann)
			continue
		}
		p.metrics.IdxAnnouncementsDuplicate.Add(ctx, 1)
	}
	if logger.Enabled(ctx, slog.LevelDebug) && len(unprocessed) > 0 {
		ctx = slogctx.Append(
			ctx,
			slog.Any(constants.UnprocessedAnnouncements, unprocessed[:min(2, len(unprocessed))]),
		)
	}
	if len(unprocessed) == 0 {
		logger.InfoContext(ctx, "no new announcements")
		span.AddEvent("no new announcements")
		return nil
	}

	logger.DebugContext(ctx, "saving announcements")
	span.AddEvent("saving announcements")
	if err := p.saveAnnouncements(ctx, unprocessed); err != nil {
		return fmt.Errorf("saving announcements: %w", err)
	}
	logger.InfoContext(ctx, "saved announcements")
	span.AddEvent("saved announcements")
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
	p.metrics.IdxAnnouncementsSaved.Add(ctx, int64(len(announcements)))
	logger.InfoContext(ctx, "inserted announcements")
	span.AddEvent("inserted announcements")

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
	logger.DebugContext(ctx, "inserted attachments")
	span.AddEvent("inserted attachments")

	return nil
}
