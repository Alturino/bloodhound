-- cleanup-non-pdf-attachments.sql
-- Beads: bloodhound-9h3
-- One-off cleanup of attachment rows whose file is not a PDF.
-- Pre-existing rows in the `attachments` table from before the PDF filter
-- landed (see bloodhound-x4y) still include .zip, .xlsx, etc. This script
-- removes them.
--
-- Schema note: the attachments table has no PDFFilename column. The PDF
-- column produced by the ingestion filter is `filename` (with
-- `original_filename` and `idx_url` as the upstream identifiers). We match
-- case-insensitively against all three so a row is kept if ANY of them
-- ends in .pdf.
--
-- Run with:
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f scripts/cleanup-non-pdf-attachments.sql
--
-- This script is destructive. Take a backup first if you need to roll back.

BEGIN;

-- 1) Baseline row count.
SELECT 'before' AS phase, count(*) AS attachments_total FROM attachments;

-- 2) Preview rows that WILL be deleted (dry-run summary by extension).
SELECT
    'would_delete' AS phase,
    coalesce(lower(right(coalesce(filename, original_filename, idx_url), 6)), '(none)') AS ext_hint,
    count(*) AS rows
FROM attachments
WHERE NOT (
    filename          ILIKE '%.pdf'
    OR original_filename ILIKE '%.pdf'
    OR idx_url           ILIKE '%.pdf'
)
GROUP BY 2
ORDER BY rows DESC;

-- 3) Delete non-PDF rows.
DELETE FROM attachments
WHERE NOT (
    filename          ILIKE '%.pdf'
    OR original_filename ILIKE '%.pdf'
    OR idx_url           ILIKE '%.pdf'
);

-- 4) Post-delete counts.
SELECT 'after' AS phase, count(*) AS attachments_total FROM attachments;

-- 5) Sanity check: must be zero. If this returns > 0, the filter logic
--    above is wrong; ROLLBACK manually before COMMIT takes effect.
SELECT
    'leftover_non_pdf' AS phase,
    count(*) AS rows
FROM attachments
WHERE NOT (
    filename          ILIKE '%.pdf'
    OR original_filename ILIKE '%.pdf'
    OR idx_url           ILIKE '%.pdf'
);

COMMIT;
