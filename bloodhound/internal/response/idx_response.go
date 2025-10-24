package response

import "time"

type IdxResponse struct {
	Replies     []Reply `json:"Replies"`
	ResultCount int     `json:"ResultCount"`
}

type Reply struct {
	Announcement Announcement `json:"pengumuman"`
	Attachments  []Attachment `json:"attachments"`
}

type Attachment struct {
	DownloadURL string `json:"FullSavePath"`
	Filename    string `json:"OriginalFilename"`
	ID          int    `json:"Id"`
}

type Announcement struct {
	Date              time.Time `json:"TglPengumuman"`
	ID                string    `json:"Id2"`
	Ticker            string    `json:"Kode_Emiten"`
	NoPengumuman      string    `json:"NoPengumuman"`
	Title             string    `json:"JudulPengumuman"`
	PerihalPengumuman string    `json:"PerihalPengumuman"`
}
