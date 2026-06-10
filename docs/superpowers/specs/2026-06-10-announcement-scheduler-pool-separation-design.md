# Announcement Scheduler & Pool Separation

## Problem

`internal/idx/announcement_worker.go` contains a monolithic `AnnouncementScheduler` that combines three responsibilities:
- **Scheduling** — periodic ticker loop to trigger work
- **Fetching** — API pagination, page fetching, semaphore limiting
- **Processing** — deduplication, DB insertion

This makes the file large (425 lines), couples concerns, and diverges from the existing `attachment_scheduler.go` / `attachment_pool.go` pattern.

## Design

Split into two files following the attachment pattern:

### `announcement_pool.go` — unexported `announcementPool`

| Aspect | Detail |
|--------|--------|
| Responsibility | Manage N goroutine workers that process announcement pages from a channel |
| Fields | `taskChan chan *Page`, `workerCount`, stores, logger/tracer/metrics, ctx/cancel |
| Constructor | `NewAnnouncementPool(workerCount, logger, tracer, metrics, announcementStore, attachmentStore, db) *announcementPool` |
| Start | Spawn N worker goroutines |
| Submit | Non-blocking send to `taskChan` |
| Worker | `workerLoop(id)` reads channel, calls `processPage` + `saveAnnouncements` |
| Shutdown | Cancel context, close channel |

### `announcement_scheduler.go` — exported `AnnouncementScheduler`

| Aspect | Detail |
|--------|--------|
| Responsibility | Run periodic ticker, fetch announcement pages from IDX API, submit pages to pool |
| Fields | `config *config.Scheduler`, `pool *announcementPool`, stores, client, db, semaphore, tracer/metrics/logger, ctx/cancel, `pageSize` |
| Constructor | `NewAnnouncementScheduler(ctx, cfg *config.Scheduler, pageSize, workerCount int, stores...) *AnnouncementScheduler` |
| Start | Run seed `process()`, start pool, start schedule goroutine |
| Schedule | `time.Tick(cfg.Interval)` loop, calls `process()` on tick |
| Process | `process()` → `processAnnouncements()` → `getAndSubmitPage()` with semaphore |
| Shutdown | Cancel context, pool.Shutdown() |

## Config Source

The scheduler uses `config.Scheduler` (top-level, same as attachment scheduler), **not** the nested `config.IDX.WorkerPool.Scheduler`.

## Files Changed

| File | Action |
|------|--------|
| `internal/idx/announcement_worker.go` | **Deleted** — content split into pool + scheduler |
| `internal/idx/announcement_pool.go` | **Created** — worker pool |
| `internal/idx/announcement_scheduler.go` | **Created** — scheduler |
| `cmd/idx_run.go` | Updated constructor call |
| `cmd/idx_run_ann.go` | Updated constructor call |

## Constructor Migration

```diff
- NewIdxAnnouncementScheduler(ctx, config *config.IDX, ...)
+ NewAnnouncementScheduler(ctx, cfg *config.Scheduler, pageSize, workerCount int, stores...)
```

## Pool Task Model

Pool accepts `*Page` tasks (same as current). Worker goroutines process announcements from the page: deduplicate via `IsProcessed`, insert new announcements, insert attachments.

## Key Decisions

- **Pool is unexported** — same as `attachmentPool`, prevents external misuse
- **Semaphore stays in scheduler** — it gates API fetch concurrency, not processing
- **Seeding preserved** — `Start()` runs `process()` once for warm start
- **Storage not needed** — `blobstorage.Storage` was passed but never used in announcement processing; removed from pool
