package idx

import (
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/state"
)

type IDXConfig struct {
	Config         *config.Config
	Logger         *slog.Logger
	Tracer         trace.Tracer
	Store          state.IdxStore
	AttachmentPool *AttachmentPool
}
