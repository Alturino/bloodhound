package idx

import (
	"log/slog"
	"time"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

type AnnouncementResponse struct {
	ResultCount   int            `json:"result_count"`
	SearchParams  SearchParams   `json:"search_params"`
	Announcements []Announcement `json:"replies"`
}

type SearchParams struct {
	DateFrom   string `json:"date_from"`
	DateTo     string `json:"date_to"`
	Query      string `json:"query"`
	Language   string `json:"language"`
	StockCode  string `json:"stock_code"` // KodeEmiten
	StockType  string `json:"stock_type"` // EmitenType
	SortOrder  string `json:"sort_order"`
	SortColumn string `json:"sort_column"`
	IndexFrom  int    `json:"index_from"`
	PageSize   int    `json:"page_size"`
}

type Announcement struct {
	ID                string       `json:"id"`
	AnnouncementTitle string       `json:"announcement_title"` // JudulPengumuman
	AnnouncementType  string       `json:"announcement_type"`  // JenisPengumuman
	StockCode         string       `json:"stock_code"`         // Kode_Emiten
	IsStock           bool         `json:"is_stock"`           // EfekEmiten_Saham
	Date              time.Time    `json:"announcement_date"`  // TglPengumuman
	CreatedDate       time.Time    `json:"created_date"`
	Attachments       []Attachment `json:"attachments"`
}

func (a Announcement) ToAnnouncement() model.Announcements {
	return model.Announcements{
		IdxID:     a.ID,
		StockCode: a.StockCode,
		Title:     a.AnnouncementTitle,
		Date:      a.Date,
		CreatedAt: a.CreatedDate,
	}
}

func (a Announcement) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", a.ID),
		slog.String("title", a.AnnouncementTitle),
		slog.String("stock_code", a.StockCode),
		slog.Bool("is_stock", a.IsStock),
		slog.Time("date", a.Date),
		slog.Time("created_date", a.CreatedDate),
	)
}

func (a Announcement) ToAttachments() []model.Attachments {
	attachments := make([]model.Attachments, 0, len(a.Attachments))
	for _, att := range a.Attachments {
		if !isPDF(att) {
			continue
		}
		attachments = append(attachments, att.ToAttachments(a.ToAnnouncement()))
	}
	return attachments
}
