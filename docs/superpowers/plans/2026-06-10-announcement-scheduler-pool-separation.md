# Announcement Scheduler & Pool Separation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the monolithic `AnnouncementScheduler` in `announcement_worker.go` into `announcement_pool.go` (worker pool) and `announcement_scheduler.go` (scheduler with ticker), following the existing `attachment_pool.go` / `attachment_scheduler.go` pattern.

**Architecture:** Create an unexported `announcementPool` struct handling page processing workers, and an exported `AnnouncementScheduler` struct that owns the pool and the scheduling ticker. The scheduler uses `config.Scheduler` (top-level) for the interval. The old `announcement_worker.go` file is deleted.

**Tech Stack:** Go 1.25, slog, OpenTelemetry, sql.DB

---

### Task 1: Create `announcement_pool.go`

**Files:**
- Create: `internal/idx/announcement_pool.go`
- Delete: `internal/idx/announcement_worker.go` (after Task 3)

The unexported `announcementPool` owns:
- A channel of `*Page` tasks
- N worker goroutines that read from the channel
- `processPage` and `saveAnnouncements` (moved from the old `AnnouncementScheduler`)

- [ ] **Step 1: Create the pool struct and constructor**

Write `internal/idx/announcement_pool.go`:

```go
package idx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

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
	db                *sql.DB
}

func NewAnnouncementPool(
	workerCount int,
	logger *slog.Logger,
	tracer trace.Tracer,
	metrics *telemetry.Metrics,
	announcementStore AnnouncementStore,
	attachmentStore AttachmentStore,
	db *sql.DB,
) *announcementPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &announcementPool{
		logger:            logger,
		tracer:            tracer,
		metrics:           metrics,
		announcementStore: announcementStore,
		attachmentStore:   attachmentStore,
		db:                db,
		taskChan:          make(chan *Page, workerCount*10),
		ctx:               ctx,
		cancel:            cancel,
		workerCount:       workerCount,
	}
}
```

- [ ] **Step 2: Add pool Start, Submit, Shutdown methods**

Append to `internal/idx/announcement_pool.go`:

```go
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
```

- [ ] **Step 3: Add workerLoop method**

Append to `internal/idx/announcement_pool.go`:

```go
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
```

- [ ] **Step 4: Add processPage method (extracted from old AnnouncementScheduler)**

Append to `internal/idx/announcement_pool.go`:

```go
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
	processedMap, err := p.announcementStore.IsProcessed(ctx, p.db, idxIDs...)
	if err != nil {
		return err
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
			return err
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
		ctx = slogctx.Append(ctx, slog.Any(constants.UnprocessedAnnouncements, unprocessed[:min(2, len(unprocessed))]))
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
```

- [ ] **Step 5: Add saveAnnouncements method (extracted from old AnnouncementScheduler)**

Append to `internal/idx/announcement_pool.go`:

```go
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
	if err := p.announcementStore.InsertAnnouncement(
		ctx,
		p.db,
		modelAnnouncements...,
	); err != nil {
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
		if err := p.attachmentStore.InsertAttachment(ctx, p.db, attachmentChunks...); err != nil {
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
```

Note: The `slices` import from the old file is needed. Add `"slices"` to the import block.

- [ ] **Step 6: Verify the file compiles**

Run: `go vet ./internal/idx/announcement_pool.go`
Expected: no errors

- [ ] **Step 7: Run existing tests to confirm nothing broke**

Run: `go test ./internal/idx/... -run TestIsPDF|TestToAttachments|TestConvertToModel -v`
Expected: all 3 test functions pass

---

### Task 2: Create `announcement_scheduler.go`

**Files:**
- Create: `internal/idx/announcement_scheduler.go`

The exported `AnnouncementScheduler` owns:
- `config *config.Scheduler` for the interval
- An internal `*announcementPool`
- The `schedule()` ticker loop
- The `process()`, `processAnnouncements()`, `getAndSubmitPage()` fetch logic

- [ ] **Step 1: Create the scheduler struct, constructor, and Start/Shutdown**

Write `internal/idx/announcement_scheduler.go`:

```go
package idx

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
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
	ctx               context.Context
	cancel            context.CancelFunc
	db                *sql.DB
	tracer            trace.Tracer
	metrics           *telemetry.Metrics
	client            Client
	sem               *semaphore.Weighted
	pageSize          int
	announcementStore AnnouncementStore
	attachmentStore   AttachmentStore
	pool              *announcementPool
}

func NewAnnouncementScheduler(
	ctx context.Context,
	cfg *config.Scheduler,
	pageSize int,
	workerCount int,
	logger *slog.Logger,
	tracer trace.Tracer,
	metrics *telemetry.Metrics,
	announcementStore AnnouncementStore,
	attachmentStore AttachmentStore,
	client Client,
	db *sql.DB,
) *AnnouncementScheduler {
	ctx, cancel := context.WithCancel(ctx)

	pool := NewAnnouncementPool(
		workerCount,
		logger,
		tracer,
		metrics,
		announcementStore,
		attachmentStore,
		db,
	)

	return &AnnouncementScheduler{
		ctx:               ctx,
		cancel:            cancel,
		config:            cfg,
		logger:            logger,
		tracer:            tracer,
		metrics:           metrics,
		client:            client,
		sem:               semaphore.NewWeighted(int64(workerCount)),
		pageSize:          pageSize,
		db:                db,
		announcementStore: announcementStore,
		attachmentStore:   attachmentStore,
		pool:              pool,
	}
}

func (s *AnnouncementScheduler) Start() {
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
	s.cancel()
	s.pool.Shutdown()
	s.logger.Info("shutdown announcement scheduler")
}
```

Note: `blobstorage.Storage` was passed to the old constructor but never used in announcement processing. It is removed from the new constructor and both cmd files are updated accordingly.

- [ ] **Step 2: Add schedule method**

Append to `internal/idx/announcement_scheduler.go`:

```go
func (s *AnnouncementScheduler) schedule() {
	interval := s.config.Interval
	logger := s.logger.With(
		slog.String("tag", "idx.AnnouncementScheduler.schedule"),
		slog.Duration(constants.Interval, interval),
	)

	ticker := time.Tick(interval)
	for {
		select {
		case <-s.ctx.Done():
			logger.InfoContext(s.ctx, "context done, stopping")
			return
		case t := <-ticker:
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
```

- [ ] **Step 3: Add process method**

Append to `internal/idx/announcement_scheduler.go`:

```go
func (s *AnnouncementScheduler) process(ctx context.Context) error {
	start := time.Now()
	defer func() {
		s.metrics.IdxProcessingDuration.Record(
			ctx,
			float64(time.Since(start).Milliseconds()),
			metric.WithAttributes(attribute.String(constants.Service, "idx")),
		)
	}()

	ctx, span := s.tracer.Start(ctx, "idx.AnnouncementScheduler.process",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "idx.AnnouncementScheduler.process"))

	logger.DebugContext(ctx, "get latest announcement")
	span.AddEvent("get latest announcement")
	latestAnnouncement, err := s.announcementStore.LatestAnnouncement(ctx, nil)
	if err != nil {
		latestAnnouncement.Date = time.Time{}
	}
	ctx = slogctx.Append(ctx, slog.Time(constants.LatestAnnouncementDate, latestAnnouncement.Date))
	logger.DebugContext(ctx, "got latest announcement")
	span.AddEvent("got latest announcement")

	logger.DebugContext(ctx, "processing announcements")
	span.AddEvent("processing announcements")
	if err := s.processAnnouncements(ctx, latestAnnouncement.Date); err != nil {
		return err
	}
	logger.InfoContext(ctx, "processed announcements")
	span.AddEvent("processed announcements")

	return nil
}
```

- [ ] **Step 4: Add processAnnouncements and getAndSubmitPage methods**

Append to `internal/idx/announcement_scheduler.go`:

```go
func (s *AnnouncementScheduler) processAnnouncements(ctx context.Context, since time.Time) error {
	ctx, span := s.tracer.Start(ctx, "idx.AnnouncementScheduler.processAnnouncements",
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

	if pageTotal < 0 {
		logger.InfoContext(ctx, "no pages to process")
		span.AddEvent("no pages to process")
		return nil
	}

	for curr := pageTotal - 1; curr >= 0; curr-- {
		ctx := slogctx.Append(ctx, slog.Int(constants.PageIdx, curr))
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "context done, stop sending page")
			span.AddEvent("context done, stop sending page")
			return nil
		default:
			logger.DebugContext(ctx, "get and submit announcements")
			span.AddEvent("get and submit announcements")
			go func(ctx context.Context, curr, pageTotal int, since time.Time, resp AnnouncementResponse) {
				if err := s.getAndSubmitPage(ctx, curr, pageTotal, since, resp); err != nil {
					logger.ErrorContext(ctx, "getAndSubmitPage", slog.Any("error", err))
					return
				}
			}(ctx, curr, pageTotal, since, resp)
		}
	}

	logger.DebugContext(ctx, "processed announcements")
	span.AddEvent("processed announcements")
	return nil
}

func (s *AnnouncementScheduler) getAndSubmitPage(
	ctx context.Context,
	curr, pageTotal int,
	since time.Time,
	resp AnnouncementResponse,
) error {
	ctx, span := s.tracer.Start(ctx, "idx.AnnouncementScheduler.getAndSubmitPage",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.Int(constants.PageIdx, curr),
			attribute.Int(constants.PageTotal, pageTotal),
		),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "idx.AnnouncementScheduler.getAndSubmitPage"))

	logger.DebugContext(ctx, "semaphore acquire")
	span.AddEvent("semaphore acquire")
	if err := s.sem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("semaphore acquire: %w", err)
	}
	defer s.sem.Release(1)
	logger.DebugContext(ctx, "semaphore acquired")
	span.AddEvent("semaphore acquired")

	fetchStart := time.Now()
	resp, err := s.client.FetchAnnouncements(ctx, curr, since)
	if err != nil {
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
	logger.InfoContext(ctx, "submitted page")
	span.AddEvent("submitted page")

	return nil
}
```

- [ ] **Step 5: Verify the file compiles**

Run: `go vet ./internal/idx/announcement_scheduler.go`
Expected: no errors

---

### Task 3: Delete `announcement_worker.go` and update cmd files

**Files:**
- Delete: `internal/idx/announcement_worker.go`
- Modify: `cmd/idx_run.go`
- Modify: `cmd/idx_run_ann.go`

- [ ] **Step 1: Delete the old monolithic file**

Run: `rm internal/idx/announcement_worker.go`

- [ ] **Step 2: Update `cmd/idx_run.go` constructor call**

Replace the `NewIdxAnnouncementScheduler` call with `NewAnnouncementScheduler`:

In `cmd/idx_run.go`, change:

```go
	idxWorker := idx.NewIdxAnnouncementScheduler(
		ctx,
		&cfg.App.IDX,
		logger.With(slog.String("tag", "idx.Worker")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.annStore,
		deps.attStore,
		deps.client,
		deps.db,
		deps.stg,
	)
	defer idxWorker.Shutdown()
```

to:

```go
	announcementScheduler := idx.NewAnnouncementScheduler(
		ctx,
		cfg.Scheduler,
		cfg.App.IDX.PageSize,
		cfg.App.IDX.WorkerPool.AnnouncementWorkers,
		logger.With(slog.String("tag", "idx.AnnouncementScheduler")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.annStore,
		deps.attStore,
		deps.client,
		deps.db,
	)
	defer announcementScheduler.Shutdown()
```

- [ ] **Step 3: Update `cmd/idx_run_ann.go` constructor call**

Similarly replace the old call in `cmd/idx_run_ann.go`:

```go
	idxWorker := idx.NewIdxAnnouncementScheduler(
		ctx,
		&cfg.App.IDX,
		deps.logger.With(slog.String("tag", "idx.Worker")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.annStore,
		deps.attStore,
		deps.client,
		deps.db,
		deps.stg,
	)
	defer idxWorker.Shutdown()
```

to:

```go
	announcementScheduler := idx.NewAnnouncementScheduler(
		ctx,
		cfg.Scheduler,
		cfg.App.IDX.PageSize,
		cfg.App.IDX.WorkerPool.AnnouncementWorkers,
		deps.logger.With(slog.String("tag", "idx.AnnouncementScheduler")),
		deps.tmt.Tracer,
		deps.tmt.Metrics,
		deps.annStore,
		deps.attStore,
		deps.client,
		deps.db,
	)
	defer announcementScheduler.Shutdown()
```

- [ ] **Step 4: Full project build check**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 5: Run all tests**

Run: `go test ./internal/idx/... -v`
Expected: all existing tests pass

---

### Post-Separation: Store cleanup

After the deletion, verify no files import from `announcement_worker.go` or reference old constructor:

- [ ] **Step 1: Verify no stale references**

Run: `rg "NewIdxAnnouncementScheduler" . --type go`
Expected: no results (all references migrated)
