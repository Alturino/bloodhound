package response

import (
	"github.com/Alturino/bloodhound/internal/common"
)

type Response struct {
	Replies     []Reply `json:"Replies"`
	ResultCount int     `json:"ResultCount"`
}

type Reply struct {
	Pengumuman  Pengumuman   `json:"pengumuman"`
	Attachments []Attachment `json:"attachments"`
}

type Attachment struct {
	FullSavePath     string `json:"FullSavePath"`
	PDFFilename      string `json:"PDFFilename"`
	OriginalFilename string `json:"OriginalFilename"`
	ID               int    `json:"Id"`
}

type Pengumuman struct {
	TglPengumuman     common.LocalTime `json:"TglPengumuman"`
	KodeEmiten        string           `json:"Kode_Emiten"`
	NoPengumuman      string           `json:"NoPengumuman"`
	JudulPengumuman   string           `json:"JudulPengumuman"`
	PerihalPengumuman string           `json:"PerihalPengumuman"`
	OldFinalID        int              `json:"OldFinalId"`
}
