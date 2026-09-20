package stockbit

import (
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/config"
	"github.com/alturino/bloodhound/internal/store"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type Config struct {
	Config        *config.Config
	Logger        *slog.Logger
	Tracer        trace.Tracer
	Metrics       *telemetry.MetricsProvider
	StockbitStore store.StockbitStore
}
