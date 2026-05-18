# blobstorage.localFile Test Specification

## Overview

This document specifies test cases for the `localFile` type in the `blobstorage` package. The tests follow table-driven patterns established in this codebase.

## localFile Type

```go
type localFile struct {
    config *config.Local
    logger *slog.Logger
    tracer trace.Tracer
}
```

### Dependencies

- `config.Local`: Contains `Enabled bool` and `BloodhoundDir string`
- Uses `slog.Logger` for logging
- Uses OpenTelemetry `trace.Tracer` for tracing

### Constructor

```go
func NewLocalFile(config *config.Local, logger *slog.Logger, tracer trace.Tracer) Storage
```

- Returns `Storage` interface
- Returns `*noopLocalFile` if `config.Enabled` is false
- Returns `*noopLocalFile` if `CreateBucket` fails

## Methods Under Test

### 1. SaveReader

```go
func (f *localFile) SaveReader(
    ctx context.Context,
    filename string,
    content io.Reader,
    contentSize int64,
    contentType string,
) (SaveResult, error)
```

**Behavior:**
- Creates directory at `filepath.Join(config.BloodhoundDir, filepath.Dir(filename))`
- Uses `filepath.Base(filename)` for final filename
- Creates file with mode `0o755`
- Computes SHA256 hash of content
- Returns `SaveResult` with `UploadInfo` containing bucket and checksum

**Success scenarios:**
- Write new file with valid content
- Write file in nested directory (creates parent dirs)
- Overwrite existing file

**Failure scenarios:**
- Directory creation fails (permissions)
- File creation fails (permissions, disk full)
- Write fails (disk error)

### 2. Exists

```go
func (f *localFile) Exists(ctx context.Context, filename string) (bool, error)
```

**Behavior:**
- Joins `config.BloodhoundDir` with `filepath.Clean(filename)`
- Returns `(false, nil)` if file doesn't exist
- Returns `(true, nil)` if file exists and size > 0
- Returns `(false, err)` on other errors (permission denied, etc.)

**Success scenarios:**
- File exists with size > 0
- File doesn't exist

**Failure scenarios:**
- Permission denied checking file
- Path is a directory instead of file

### 3. Download

```go
func (f *localFile) Download(ctx context.Context, object string) (io.ReadCloser, error)
```

**Behavior:**
- Joins `config.BloodhoundDir` with `filepath.Clean(object)`
- Reads entire file into memory
- Returns `io.NopCloser` wrapping `bytes.Reader`

**Success scenarios:**
- Read existing file
- Read file with binary content
- Read empty file

**Failure scenarios:**
- File doesn't exist
- Permission denied
- Path is directory

### 4. CreateBucket

```go
func (f *localFile) CreateBucket(ctx context.Context) error
```

**Behavior:**
- Creates `config.BloodhoundDir` with mode `0o755`
- Returns nil if directory already exists

**Success scenarios:**
- Create new directory
- Directory already exists (idempotent)

**Failure scenarios:**
- Permission denied creating directory
- Parent directory creation fails

## Edge Cases

### Permissions
- Read-only directory (SaveReader fails on mkdir)
- No read permission on file (Exists fails)
- No read permission on parent dir (Download fails)
- No write permission (SaveReader fails on create)

### Missing Directories
- Empty BloodhoundDir path
- Non-existent parent directories in filename
- Deep nested paths

### Invalid Paths
- Path traversal attempts (`../../../etc/passwd`)
- Empty filename
- Filename with only slashes
- Absolute paths in filename

### Concurrent Access
- Multiple goroutines writing same file
- Read while writing
- Delete while reading

### File Edge Cases
- Zero-byte file (Exists should return false)
- Very large file (memory handling)
- File deleted between Exists and Download
- File modified between Exists and Download

## Table-Driven Test Patterns

Follow patterns from `internal/idx/attachment_pool_test.go` and `internal/idx/announcement_pool_test.go`:

### Structure

```go
func TestLocalFile_MethodName(t *testing.T) {
    tests := []struct {
        name     string
        setup    func(*testing.T) (localFile, context.Context, func())
        run      func(*testing.T, localFile, context.Context) error
        check    func(*testing.T, error)
    }{
        // cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            lf, ctx, cleanup := tt.setup(t)
            defer cleanup()
            err := tt.run(t, lf, ctx)
            tt.check(t, err)
        })
    }
}
```

### Test Fixtures

- Use `t.TempDir()` for isolated test directories
- Use `os.CreateTemp()` for test files
- Cleanup via `defer os.RemoveAll()` or `defer cleanup()`

### Fake Workers

Reference pattern from `fakeAttachmentWorker` in test files:
- Use channels for async notification
- Use mutex for shared state
- Use atomic for counters

### Error Testing

Use sentinel error type like `assertError`:
```go
type assertError struct{}
func (e assertError) Error() string { return "assert error for testing" }
```

### Timeouts in Tests

- Use `time.Sleep()` for async operations (e.g., `100ms`)
- Use `select` with `time.After()` for timeout detection

## Existing Tests

**No existing tests for localFile.** There is a `Fake` implementation in `fake.go` for use in other tests, and a `noopLocalFile` in `noopfile.go` for disabled storage.

## Recommended Test File Structure

Create `internal/blobstorage/localfile_test.go`:

### Test Cases to Implement

1. **TestLocalFile_NewLocalFile**
   - Enabled=true with valid dir
   - Enabled=false returns noopLocalFile
   - CreateBucket failure returns noopLocalFile

2. **TestLocalFile_SaveReader**
   - Success: new file, nested dirs, overwrite
   - Fail: mkdir fails, create fails, write fails

3. **TestLocalFile_Exists**
   - True: file exists with size > 0
   - False: file doesn't exist
   - False: file exists but empty (size 0)
   - Error: permission denied

4. **TestLocalFile_Download**
   - Success: read file content, binary content
   - Fail: file doesn't exist, permission denied

5. **TestLocalFile_CreateBucket**
   - Success: new dir, dir already exists
   - Fail: permission denied

6. **TestLocalFile_PathTraversal** (security)
   - Prevent `../../../etc/passwd` style attacks

7. **TestLocalFile_Concurrent** (optional)
   - Multiple goroutines writing same file

## Benchmark Recommendations

Create `internal/blobstorage/localfile_bench_test.go`:

- Benchmark SaveReader with various content sizes
- Benchmark Exists with cold/warm cache
- Benchmark Download with various file sizes

Use `slog.New(slog.DiscardHandler)` for benchmark logging (see `attachment_pool_bench_test.go`).

## Dependencies for Tests

```go
import (
    "bytes"
    "context"
    "io"
    "log/slog"
    "os"
    "path/filepath"
    "testing"

    "go.opentelemetry.io/otel/trace/noop"

    "github.com/alturino/bloodhound/config"
)
```