package idx

import (
	"context"

	"github.com/alturino/bloodhound/internal/models"
)

// AnnouncementPool processes announcements in parallel using worker pool pattern
type AnnouncementPool interface {
	Pool[AnnouncementTask]
	// ProcessPage processes all announcements for a given page
	ProcessPage(
		ctx context.Context,
		page int,
		announcements []models.Announcement,
	) ([]AnnouncementResult, error)
}
