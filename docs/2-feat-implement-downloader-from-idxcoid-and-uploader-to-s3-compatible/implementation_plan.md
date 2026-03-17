# Automated IDX Announcement Downloader and S3 Uploader

Automate the ingestion of corporate announcements from IDX by polling their API, downloading attachments, and archiving them to S3-compatible storage with a standardized naming convention.

## Proposed Changes

### Storage Component
Define an interface for storage to support different S3-compatible providers (AWS, MinIO, etc.).

#### [NEW] [storage.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/storage/storage.go)
- Define `Storage` interface with `Upload`, `Exists`, and `Download` methods.

#### [NEW] [minio.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/storage/minio.go)
- Implement `MinIOStorage` using the [minio-go](https://github.com/minio/minio-go) library.

### State Management
Track the last processed announcement to avoid duplicate work and redundant API polling.

#### [NEW] [store.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/state/store.go)
- Define `Store` interface for state persistence (e.g., `GetLastProcessed()`, `SetLastProcessed()`).
- Implement a simple file-based store (JSON or YAML) in `file_store.go`.

### Worker & Orchestration
A background service that ties everything together.

#### [NEW] [worker.go](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/internal/worker/worker.go)
- Implement the polling loop.
- Handle downloading attachments from IDX URLs.
- Implement naming logic: `yyyy-MM-dd_KODE_EMITEN_checksum_original_filename.ext`.
    - `yyyy-MM-dd`: Derived from `TglPengumuman`.
    - `KODE_EMITEN`: Preserved as is (uppercase).
    - `checksum`: SHA256 hash (first 8 characters) of the file content.
    - `original_filename`: Lowercased.
- Orchestrate deduplication check before upload.

### Configuration
Update configuration to include MinIO credentials and polling interval.

#### [MODIFY] [config.yaml](file:///home/onirutla/Personal/coding/projects/bloodhound/feat/downloader_uploader/config.yaml)
- Add `minio` section: `endpoint`, `bucket`, `access_key`, `secret_key`, `use_ssl`.
- Add `worker` section: `interval`.

## Verification Plan

### Automated Tests
- Unit tests for the refined renaming logic.
- Unit tests for deduplication logic with mocked MinIO client.
- Integration test for IDX API client (existing).

### Manual Verification
- Run the worker and verify logs.
- Check MinIO bucket for correctly named files after a poll cycle.
