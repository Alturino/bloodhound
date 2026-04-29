package idx

import (
	"time"

	"github.com/alturino/bloodhound/internal/models"
)

type AttachmentTask struct {
	TotalAttachment   int               `json:"total_attachment"`
	AttachmentItem    int               `json:"attachment_item"`
	AnnouncementID    string            `json:"id2"`
	AnnouncementTitle string            `json:"announcement_title"` // JudulPengumuman
	StockCode         string            `json:"stock_code"`         // Kode_Emiten
	Date              time.Time         `json:"announcement_date"`  // TglPengumuman
	Attachment        models.Attachment `json:"attachment"`
}
