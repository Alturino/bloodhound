package worker

import (
	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
	"github.com/alturino/bloodhound/pkg/idx"
	"github.com/alturino/bloodhound/pkg/stockbit"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
)

type WorkerConfig struct {
	Config        *config.Config
	Logger        *slog.Logger
	Tracer        trace.Tracer
	IdxStore      state.IdxStore
	StockbitStore state.StockbitStore
}

func NewWorkerIdx(cfg *WorkerConfig, idxClient idx.Client, storage storage.Storage) *WorkerIdx {
	return &WorkerIdx{
		config:    cfg.Config,
		logger:    cfg.Logger,
		tracer:    cfg.Tracer,
		idxClient: idxClient,
		storage:   storage,
		idxStore:  cfg.IdxStore,
	}
}

func NewWorkerStockbit(cfg *WorkerConfig, stockbitClient stockbit.Client) *WorkerStockbit {
	return &WorkerStockbit{
		config:         cfg.Config,
		logger:         cfg.Logger,
		tracer:         cfg.Tracer,
		stockbitClient:  stockbitClient,
		stockbitStore:   cfg.StockbitStore,
	}
}