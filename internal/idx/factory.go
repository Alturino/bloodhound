package idx

import (
	"github.com/alturino/bloodhound/internal/storage"
)

func NewWorkerIdx(
	config *IDXConfig,
	client Client,
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


