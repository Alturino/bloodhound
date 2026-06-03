#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<EOSQL
  -- pdf_extractor: processes attachments, needs read/write on attachments and announcements
  CREATE USER bloodhound_pdf_extractor WITH PASSWORD '${POSTGRES_BLOODHOUND_PDF_EXTRACTOR_PASSWORD}';
  GRANT CONNECT ON DATABASE bloodhound TO bloodhound_pdf_extractor;
  GRANT USAGE ON SCHEMA public TO bloodhound_pdf_extractor;
  GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO bloodhound_pdf_extractor;
  ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE ON TABLES TO bloodhound_pdf_extractor;

  -- analyzer: read/write access for analysis workloads
  CREATE USER bloodhound_analyzer WITH PASSWORD '${POSTGRES_BLOODHOUND_ANALYZER_PASSWORD}';
  GRANT CONNECT ON DATABASE bloodhound TO bloodhound_analyzer;
  GRANT USAGE ON SCHEMA public TO bloodhound_analyzer;
  GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO bloodhound_analyzer;
  ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE ON TABLES TO bloodhound_analyzer;
EOSQL
