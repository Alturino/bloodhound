package response

import (
	"github.com/Alturino/bloodhound/internal/common"
)

type Response struct {
	Replies     []Reply `json:"Replies"`
	ResultCount int     `json:"ResultCount"`
}

type Reply struct {
	Attachments []Attachment `json:"attachments"`
	Pengumuman  Pengumuman   `json:"pengumuman"`
}

type Attachment struct {
	FullSavePath string `json:"FullSavePath"`
	ID           int    `json:"Id"`
	PDFFilename  string `json:"PDFFilename"`
}

type Pengumuman struct {
	KodeEmiten        string           `json:"Kode_Emiten"`
	NoPengumuman      string           `json:"NoPengumuman"`
	OldFinalID        int              `json:"OldFinalId"`
	PerihalPengumuman string           `json:"PerihalPengumuman"`
	TglPengumuman     common.LocalTime `json:"TglPengumuman"`
}
