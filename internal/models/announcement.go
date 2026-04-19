package models

import (
	"time"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

// AnnouncementResponse is the top-level API response from IDX
type AnnouncementResponse struct {
	ResultCount  int          `json:"result_count"`
	SearchParams SearchParams `json:"search_params"`
	Replies      []Reply      `json:"replies"`
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

// Reply contains an announcement with its attachments
type Reply struct {
	Announcement Announcement `json:"announcement"` // pengumuman
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
	AnnouncementDate    time.Time    `json:"announcement_date"` // TglPengumuman
	CreatedDate         time.Time    `json:"created_date"`
	Attachments         []Attachment `json:"attachments"`
}

func (a Announcement) ToAnnouncements() model.Announcements {
	return model.Announcements{
		IdxID:             a.ID2,
		StockCode:         a.StockCode,
		Date:              a.AnnouncementDate,
		AnnouncementTitle: a.AnnouncementTitle,
		CreatedAt:         a.CreatedDate,
	}
}

// Attachment contains PDF attachment details
type Attachment struct {
	ID               int    `json:"id"`
	PDFFilename      string `json:"pdf_filename"`   // PDFFilename
	FullSavePath     string `json:"full_save_path"` // FullSavePath - URL to download
	JMSXGroupID      string `json:"jmsx_group_id"`
	CorrelationID    string `json:"correlation_id"`
	IsAttachment     bool   `json:"is_attachment"`
	OriginalFilename string `json:"original_filename"` // OriginalFilename
}

func (a Attachment) ToAttachments(announcementID, checksum, storagePath string) model.Attachments {
	return model.Attachments{
		IdxAnnouncementID: announcementID,
		OriginalFilename:  a.OriginalFilename,
		Checksum:          checksum,
		StoragePath:       storagePath,
	}
}
