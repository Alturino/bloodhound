package idx

import "github.com/alturino/bloodhound/internal/models"

// AnnouncementTask represents a single announcement processing task
type AnnouncementTask struct {
	Page         int
	Index        int
	Announcement models.Announcement
}
