package stockbit

import (
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type Config struct {
	Config  *config.Config
	Logger  *slog.Logger
	Tracer  trace.Tracer
	Metrics *telemetry.Metrics
	Store   Store
}
