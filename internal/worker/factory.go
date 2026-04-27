package worker

import (
	"github.com/alturino/bloodhound/internal/storage"
	"github.com/alturino/bloodhound/pkg/idx"
	"github.com/alturino/bloodhound/pkg/stockbit"
)

func NewWorkerIdx(
	config *IDXConfig,
	client idx.Client,
	storage storage.Storage,
	announcementPool AnnouncementPool,
) *IDX {
	return &IDX{
		config:           config.Config,
		logger:           config.Logger,
		tracer:           config.Tracer,
		client:           client,
		storage:          storage,
		store:            config.Store,
		announcementPool: announcementPool,
	}
}

func NewWorkerStockbit(cfg *StockbitConfig, stockbitClient stockbit.Client) *Stockbit {
	return &Stockbit{
		config:        cfg.Config,
		logger:        cfg.Logger,
		tracer:        cfg.Tracer,
		client:        stockbitClient,
		stockbitStore: cfg.StockbitStore,
	}
}
