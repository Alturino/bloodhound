#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  -v extractor_user="$POSTGRES_BLOODHOUND_PDF_EXTRACTOR_USER" \
  -v extractor_password="$POSTGRES_BLOODHOUND_PDF_EXTRACTOR_PASSWORD" \
  -v analyzer_user="$POSTGRES_BLOODHOUND_PDF_ANALYZER_USER" \
  -v analyzer_password="$POSTGRES_BLOODHOUND_PDF_ANALYZER_PASSWORD" <<EOSQL

  -- pdf_extractor
  CREATE USER :extractor_user WITH PASSWORD :'extractor_password';
  GRANT CONNECT ON DATABASE bloodhound TO :extractor_user;
  GRANT USAGE ON SCHEMA public TO :extractor_user;
  GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO :extractor_user;
  ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE ON TABLES TO :extractor_user;

  -- analyzer
  CREATE USER :analyzer_user WITH PASSWORD :'analyzer_password';
  GRANT CONNECT ON DATABASE bloodhound TO :analyzer_user;
  GRANT USAGE ON SCHEMA public TO :analyzer_user;
  GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO :analyzer_user;
  ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE ON TABLES TO :analyzer_user;
EOSQL
