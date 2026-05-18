package idx

import (
	"time"

	"github.com/alturino/bloodhound/internal/models"
)

type AttachmentTask struct {
	TtachmentItem     int               `json:"attachment_item"`
	AnnouncementID    string            `json:"announcement_id"`
	AnnouncementTitle string            `json:"announcement_title"`
	StockCode         string            `json:"stock_code"`
	Date              time.Time         `json:"announcement_date"`
	Attachment        models.Attachment `json:"attachment"`
}
