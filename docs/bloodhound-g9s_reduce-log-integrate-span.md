# Reduce Log and Integrate Log with Span Event

**Beads Issue**: `bloodhound-g9s`
**Author**: Ricky Alturino
**Date**: 2026-06-08

## Problem

Triple redundancy on every error path:

```go
err = fmt.Errorf("inserting attachments: %w", err)     // wrap
logger.ErrorContext(ctx, "inserting attachments", ...)  // log
telemetry.RecordError(span, err)                        // span error
```

And `RecordError` itself was triple-reporting:
```go
span.AddEvent(err.Error())     // (a) span event
span.SetStatus(codes.Error, ...) // (b) status
span.RecordError(err)           // (c) OTel exception event
```

No span events on process start/completion. Inconsistent attribute keys (raw strings everywhere).

## Changes

### 1. `internal/constants/constants.go` (NEW)

All attribute key constants in one place — no more raw string keys scattered across files:

- Trace attribute constants (`AnnouncementsTotal`, `PageTotal`, `Bucket`, `File`, etc.)
- HTTP attribute constants (`HTTPURL`, `HTTPMethod`, `HTTPRespStatus`, etc.)

### 2. `internal/telemetry/telemetry.go`

- **`RecordError`**: removed `span.AddEvent(err.Error())` — redundant with OTel's `span.RecordError` which already creates an exception event
- **Baggagecopy**: registered `baggagecopy.NewSpanProcessor(baggagecopy.AllowAllMembers)` in tracer provider — automatically copies baggage members as span attributes on child span creation

### 3. Error handling convention (all 11 files)

| Level | Pattern |
|---|---|
| **Lowest** (stores, minio, file, HTTP clients) | `telemetry.RecordError(span, err)` + `fmt.Errorf("ctx: %v", err)` — **no `logger.ErrorContext`** |
| **Mid** (Process, workers) | Pass annotated error up — no log |
| **Top** (scheduler, Start) | `logger.ErrorContext` with final error |

`%w` → `%v` everywhere (error already recorded via telemetry at source).

### 4. Span events on start/end (all 11 files)

- `logger.DebugContext(ctx, "doing X")` + `span.AddEvent("doing X")` — process start
- `logger.InfoContext(ctx, "done X")` + `span.AddEvent("done X")` — process completion

### 5. Baggage for cross-span propagation

Where `span.SetAttributes` was used for propagation-worthy data (pagination counts, dates, identifiers), replaced with:

```go
m, _ := baggage.NewMember(constants.Key, value)
bag, _ := baggage.FromContext(ctx).SetMember(m)
ctx = baggage.ContextWithBaggage(ctx, bag)
```

`baggagecopy` picks up the baggage and copies it as span attributes on child span creation.

`slogctx.Append` kept unchanged (log and trace are separate). `span.SetAttributes` kept for local-only attributes (HTTP response details).

## Files Modified

| File | Changes |
|---|---|
| `internal/constants/constants.go` | **NEW** — all attribute key constants |
| `internal/telemetry/telemetry.go` | `RecordError` cleanup, baggagecopy span processor |
| `internal/idx/idx.go` | ~4 redundant error logs removed, span events added, `span.SetAttributes` → baggage |
| `internal/idx/idx_httpclient.go` | Redundant error logs removed, span events, `%w`→`%v`, constants |
| `internal/idx/announcement_store.go` | Redundant error logs removed, span events, `%w`→`%v`, constants |
| `internal/idx/attachment_store.go` | Redundant error logs removed, span events, `%w`→`%v`, constants |
| `internal/idx/attachment.go` | Redundant error logs removed, span events, constants |
| `internal/idx/attachment_worker.go` | Span events, constants |
| `internal/blobstorage/minio.go` | Redundant error logs removed, span events, `%w`→`%v`, constants |
| `internal/blobstorage/file.go` | Redundant error logs removed, span events, `%w`→`%v`, constants |
| `internal/store/stockbit.go` | Redundant error logs removed, span events, `%w`→`%v`, constants |
| `internal/stockbit/worker.go` | Redundant error logs removed, span events, constants |
| `internal/stockbit/client.go` | Redundant error logs removed, span events, `%w`→`%v`, constants |
| `internal/httpclient/client.go` | Constants |
| `go.mod` / `go.sum` | Added `baggagecopy` dependency |

## Verification

1. `go build ./...` ✅
2. `go vet ./...` (pre-existing issues in `attachment_worker_test.go` only)
