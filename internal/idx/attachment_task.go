package idx

import (
	"time"
)

type AttachmentTask struct {
	AttachmentItem    int        `json:"attachment_item"`
	AnnouncementID    string     `json:"announcement_id"`
	AnnouncementTitle string     `json:"announcement_title"`
	StockCode         string     `json:"stock_code"`
	Date              time.Time  `json:"announcement_date"`
	Attachment        Attachment `json:"attachment"`
}
