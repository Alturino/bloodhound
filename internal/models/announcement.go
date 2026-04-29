package models

import (
	"log/slog"
	"time"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

// AnnouncementResponse is the top-level API response from IDX
type AnnouncementResponse struct {
	ResultCount   int            `json:"result_count"`
	SearchParams  SearchParams   `json:"search_params"`
	Announcements []Announcement `json:"replies"`
}

// SearchParams contains the search parameters used in the API request
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

// Announcement contains announcement details
type Announcement struct {
	ID2                 string       `json:"id2"`
	FinalID             string       `json:"final_id"`
	AnnouncementNumber  string       `json:"announcement_number"` // NoPengumuman
	AnnouncementTitle   string       `json:"announcement_title"`  // JudulPengumuman
	AnnouncementType    string       `json:"announcement_type"`   // JenisPengumuman
	StockCode           string       `json:"stock_code"`          // Kode_Emiten
	FormID              string       `json:"form_id"`
	AnnouncementSubject string       `json:"announcement_subject"` // PerihalPengumuman
	JMSXGroupID         string       `json:"jmsx_group_id"`
	Division            string       `json:"division"`          // Divisi
	DivisionCode        string       `json:"division_code"`     // KodeDivisi
	StockTypeDetail     string       `json:"stock_type_detail"` // JenisEmiten
	ID                  int          `json:"id"`
	OldFinalID          int          `json:"old_final_id"`
	IsStock             bool         `json:"is_stock"`          // EfekEmiten_Saham
	IsBond              bool         `json:"is_bond"`           // EfekEmiten_Obligasi
	IsEBA               bool         `json:"is_eba"`            // EfekEmiten_EBA
	IsETF               bool         `json:"is_etf"`            // EfekEmiten_ETF
	IsSPEI              bool         `json:"is_spei"`           // EfekEmiten_SPEI
	IsDIRE              bool         `json:"is_dire"`           // EfekEmiten_DIRE
	IsDINFRA            bool         `json:"is_dinfra"`         // EfekEmiten_DINFRA
	Date                time.Time    `json:"announcement_date"` // TglPengumuman
	CreatedDate         time.Time    `json:"created_date"`
	Attachments         []Attachment `json:"attachments"`
}

func (a Announcement) ToAnnouncements() model.Announcements {
	return model.Announcements{
		IdxID:             a.ID2,
		StockCode:         a.StockCode,
		Date:              a.Date,
		AnnouncementTitle: a.AnnouncementTitle,
		CreatedAt:         a.CreatedDate,
	}
}

func (a Announcement) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id2", a.ID2),
		slog.String("announcement_title", a.AnnouncementTitle),
		slog.String("stock_code", a.StockCode),
		slog.Bool("is_stock", a.IsStock),
		slog.Time("announcement_date", a.Date),
		slog.Time("created_date", a.CreatedDate),
		slog.Any("attachments", a.Attachments),
	)
}

// Attachment contains PDF attachment details
type Attachment struct {
	IsAttachment     bool   `json:"is_attachment"`
	ID               int    `json:"id"`
	PDFFilename      string `json:"pdf_filename"`   // PDFFilename
	FullSavePath     string `json:"full_save_path"` // FullSavePath - URL to download
	JMSXGroupID      string `json:"jmsx_group_id"`
	CorrelationID    string `json:"correlation_id"`
	OriginalFilename string `json:"original_filename"` // OriginalFilename
}

func (a Attachment) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("is_attachment", a.IsAttachment),
		slog.Int("id", a.ID),
		slog.String("pdf_filename", a.PDFFilename),
		slog.String("full_save_path", a.FullSavePath),
		slog.String("jmsx_group_id", a.JMSXGroupID),
		slog.String("correlation_id", a.CorrelationID),
		slog.String("original_filename", a.OriginalFilename),
	)
}

func (a Attachment) ToAttachments(announcementID, checksum, storagePath string) model.Attachments {
	return model.Attachments{
		IdxAnnouncementID: announcementID,
		OriginalFilename:  a.OriginalFilename,
		Checksum:          checksum,
		StoragePath:       storagePath,
	}
}
