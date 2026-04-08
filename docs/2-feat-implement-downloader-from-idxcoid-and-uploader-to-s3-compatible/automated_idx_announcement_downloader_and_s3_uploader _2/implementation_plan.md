# Implementation Plan - Automated IDX Announcement Downloader

Investors and researchers need real-time access to corporate announcements from the Indonesia Stock Exchange (IDX) to feed downstream AI analysis. This plan outlines the refinements needed to make the existing worker robust, efficient, and compliant with the requested naming conventions for chronological AI ingestion.

## Proposed Changes

### [Worker]

#### [MODIFY] [worker.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/worker/worker.go)
- **Refine `Process` logic**: The database is currently empty. I will implement a dual-mode logic:
    1. **Initial/Deep Scan**: If the state store is empty (or triggered via config), the worker will iterate from the last page to the first (oldest to newest) to perform a full historical seed.
    2. **Incremental Polling (Default)**: For regular cron runs, the worker will fetch page 1 and iterate through records newest to oldest, stopping once it encounters an already processed announcement. This ensures efficiency and avoids redundant API calls.
- **Update Renaming Convention**: Include `id2` to prevent collisions as requested. The new target format will be:
    `yyyy-MM-dd_{id2}_filename.*`
- **Optimize Storage Check**: Move the `Exists` check *before* the `downloadFile` call. Since the naming convention is now deterministic, we can avoid redundant downloads.
- **Graceful Error Handling**: Enhance logging with structured information (e.g., `id2`, `url`, `error_type`) without redundant error messages, ensuring clear diagnostics.

### [Models]

#### [MODIFY] [announcement.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/models/announcement.go)
- Ensure all fields from the provided JSON sample are correctly mapped, especially `Id2` for state tracking and `TglPengumuman` for the date prefix.

### [Config]

#### [MODIFY] [config.yaml](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/config.yaml)
- Verify `scheduler.interval` is set to the requested 5-15 minute range (currently 15m).

## Verification Plan

### Automated Tests
-   **Unit Tests**: Update `worker_test.go` to verify the new incremental polling logic and renaming convention.
-   **Integration Tests**: Run the worker locally against a mock IDX server (or the real one if safe) to verify successful downloads and uploads to a local MinIO instance.

### Manual Verification
-   Check the S3/MinIO bucket to ensure files are named `yyyy-MM-dd_{id2}_filename.*`.
-   Verify the database `announcements` table contains the metadata for new entries.
-   Monitor logs to verify "Initial Scan" logic correctly fills the database and subsequent "Incremental Polling" skips existing records.

## Open Questions

All previous questions have been addressed. The `id2` will be included in the filename for uniqueness.
