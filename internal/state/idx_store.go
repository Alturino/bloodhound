package state

import (
	"database/sql"
	"log/slog"

	"github.com/alturino/bloodhound/internal/telemetry"
)

func NewIdxStore(db *sql.DB, logger *slog.Logger) *DBStore {
	store := NewDBStore(db, logger)
	if store.tracer == nil {
		store.tracer = telemetry.AppTelemetry.Tracer
	}
	return store
}

func NewStockbitStore(db *sql.DB, logger *slog.Logger) *DBStore {
	store := NewDBStore(db, logger)
	if store.tracer == nil {
		store.tracer = telemetry.AppTelemetry.Tracer
	}
	return store
}