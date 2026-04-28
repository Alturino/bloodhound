package idx

import (
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/state"
	"github.com/alturino/bloodhound/internal/storage"
)

func NewWorkerIdx(
	config *config.Config,
	logger *slog.Logger,
	tracer trace.Tracer,
	store state.IdxStore,
	attachmentPool *AttachmentPool,
	client Client,
	storage storage.Storage,
	announcementPool AnnouncementPool,
) *IDX {
	return &IDX{
		config:           config,
		logger:           logger,
		tracer:           tracer,
		client:           client,
		storage:          storage,
		store:            store,
		announcementPool: announcementPool,
	}
}
