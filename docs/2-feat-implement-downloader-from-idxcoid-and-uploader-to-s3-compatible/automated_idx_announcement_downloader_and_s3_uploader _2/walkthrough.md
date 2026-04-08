# IDX Announcement Downloader - Incremental Polling & Seeding

The IDX Announcement Downloader has been enhanced with a robust, two-mode polling strategy to ensure efficient data ingestion while supporting initial cold-start scenarios.

## Changes Made

### 1. State Management & Seeding Logic
- **`HasSavedData` Check**: Added a new method to the `Store` interface to detect if the database has any saved data.
- **Initial Seeding (Bottom-Up)**: When the database is empty, the worker performs a full historical scan, fetching announcements from the last page to the first. This ensures chronological insertion into the database.
- **Incremental Polling (Top-Down)**: For subsequent runs, the worker polls starting from Page 1 and stops immediately upon encountering an announcement that has already been processed.

### 2. Standardized Naming Convention
- Updated the file renaming logic to include the `id2` field, ensuring uniqueness across all archives.
- **New Naming Format**: `yyyy-MM-dd_{id2}_{original_filename}.ext`
- **Example**: `2024-04-06_12345_annual_report.pdf`

### 3. Performance & Optimization
- **Early Storage Check**: The worker now checks for the existence of a file in S3-compatible storage **before** downloading it from the IDX server, saving significant bandwidth and processing time.
- **Structured Logging**: Improved logging with `id2` context for better observability and debugging.

### 4. Code Refactoring & Testing
- Refactored `Worker.Process` into `processInitial` and `processIncremental`.
- Implemented `HasSavedData` for `DBStore` (PostgreSQL), `FileStore` (JSON), and `FakeStore` (Testing).
- Added comprehensive unit tests in `internal/worker/worker_test.go` covering both operational modes.

## Verification Results

### Automated Tests
Ran the suite of unit tests for the worker to verify both seeding and incremental logic:
```bash
go test -v ./internal/worker/worker_test.go ./internal/worker/worker.go
```
- `TestWorker_Process_InitialSeeding`: **PASSED** (Verified reverse page fetching and ordering)
- `TestWorker_Process_IncrementalPolling`: **PASSED** (Verified early termination upon hitting seen items)

### Manual Verification
- Verified the `id2` inclusion in the naming convention logic.
- Confirmed the storage existence check logic correctly skips redundant downloads.

> [!IMPORTANT]
> The worker now relies on `id2` for both uniqueness and state tracking. Ensure that the database schema correctly indexes `id2` for performance.

> [!TIP]
> To trigger a full re-scan, you can simply empty the `announcements` table in the database; the worker will automatically switch back to "Initial Seeding" mode.
