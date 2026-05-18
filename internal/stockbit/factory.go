package stockbit

func NewWorkerStockbit(cfg *Config, stockbitClient Client) *Stockbit {
	return &Stockbit{
		config:        cfg.Config,
		logger:        cfg.Logger,
		tracer:        cfg.Tracer,
		client:        stockbitClient,
		stockbitStore: cfg.StockbitStore,
	}
}
