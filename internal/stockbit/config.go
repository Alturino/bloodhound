package stockbit

import (
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/store"
)

type Config struct {
	Config        *config.Config
	Logger        *slog.Logger
	Tracer        trace.Tracer
	StockbitStore store.StockbitStore
}
