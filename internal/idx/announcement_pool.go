package idx

import (
	"context"

	"github.com/alturino/bloodhound/internal/models"
)

// AnnouncementPool processes announcements in parallel using worker pool pattern
type AnnouncementPool interface {
	Pool[AnnouncementTask]
	// Process processes all announcements for a given page
	Process(ctx context.Context, page int, announcements []models.Announcement)
}
