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
		slog.Any("attachments", a.Attachments),
	)
}

type Attachment struct {
	PDFFilename      string `json:"pdf_filename"`      // PDFFilename
	FullSavePath     string `json:"full_save_path"`    // FullSavePath - URL to download
	OriginalFilename string `json:"original_filename"` // OriginalFilename
}

func (a Attachment) ToAttachments(ann model.Announcements) model.Attachments {
	return model.Attachments{
		IdxAnnouncementID: ann.IdxID,
		IdxURL:            a.FullSavePath,
		OriginalFilename:  a.OriginalFilename,
		Filename:          a.PDFFilename,
		Title:             ann.Title,
		StockCode:         ann.StockCode,
		Date:              ann.Date,
	}
}

func (a Announcement) ToAttachments() []model.Attachments {
	attachments := make([]model.Attachments, len(a.Attachments))
	for i, att := range a.Attachments {
		attachments[i] = att.ToAttachments(a.ToAnnouncement())
	}
	return attachments
}

func (a Attachment) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("pdf_filename", a.PDFFilename),
		slog.String("full_save_path", a.FullSavePath),
		slog.String("original_filename", a.OriginalFilename),
	)
}
