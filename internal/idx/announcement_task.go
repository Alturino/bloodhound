package idx

import (
	"context"

	"github.com/alturino/bloodhound/internal/models"
)

// AnnouncementTask represents a single announcement processing task
type AnnouncementTask struct {
	Page             int
	AnnouncementItem int
	Ctx              context.Context
	Announcement     models.Announcement
}
