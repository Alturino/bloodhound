# Refactor IDX Downloader & Database Integration

Refactor the system to use `go-jet` generated models, unify configuration structures, and optimize the worker's processing logic based on user feedback.

## User Review Required

> [!IMPORTANT]
> - **Configuration Nesting**: `IDXConfig` will now be a field within `AppConfig`.
> - **Type-Safe Database**: `DBStore` will use generated code from `internal/db/.gen/postgres/public`.
> - **PostgreSQL 18**: Custom `uuidv7()` function removed as it is now natively supported. The migration files created by the user already utilize this.

## Proposed Changes

### Configuration
#### [MODIFY] [config.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/config/config.go)
- Nest `IDXConfig` inside `AppConfig`.
- Update `viper` defaults to reflect the new path.

### State & Database
#### [MODIFY] [store.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/state/store.go)
- Ensure the interface remains compatible with updated worker requirements.

#### [MODIFY] [db_store.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/state/db_store.go)
- Use `github.com/alturino/bloodhound/internal/db/.gen/postgres/public/table` for table definitions.
- Use `github.com/alturino/bloodhound/internal/db/.gen/postgres/public/model` for type-safe record insertion and querying.
- Update `IsProcessed` to check `idx_id` (the string ID from IDX) instead of the UUID.

### Worker Logic
#### [MODIFY] [worker.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/worker/worker.go)
- Update `Worker` struct and `NewWorker` to accept `config.AppConfig`.
- Simplify the inner loop: process `Replies` in the order they are received (`0` to `len-1`) instead of reversing.
- Retain the reverse outer loop (last page to first) to ensure historical sync integrity.

### Main Entry point
#### [MODIFY] [main.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/cmd/worker/main.go)
- Update initialization logic to pass nested config.

### Test Refactoring
#### [MODIFY] [worker_test.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/worker/worker_test.go)
- Replaced `testify/mock` with manual fakes for better maintainability.
- Removed local fake implementations and moved them to their respective packages.

#### [NEW] [fake_store.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/state/fake_store.go)
- Extracted `FakeStore` to the `state` package.

#### [NEW] [fake_storage.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/storage/fake_storage.go)
- Extracted `FakeStorage` to the `storage` package.

#### [NEW] [fake_client.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/http/fake_client.go)
- Extracted `FakeClient` to the `http` package.

---

## Verification Plan

### Automated Tests
- `go test ./internal/worker/...` to ensure reverse paging logic works with the new fake-based testing infrastructure.
- `go test ./internal/state/... ./internal/storage/... ./internal/http/...` to verify the re-exported fakes are valid.

### Manual Verification
- Verified that all tests passed with `go test ./internal/worker/...`.
- Double-checked that the fakes correctly satisfy the interfaces.
