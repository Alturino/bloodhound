package response

import (
	"strings"
	"time"
)

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

type Time struct {
	time.Time
}

func (t *Time) UnmarshalJSON(b []byte) error {
	s := string(b)
	s = strings.Trim(s, "\"")
	nt, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		return err
	}
	t.Time = nt
	return nil
}

type Announcement struct {
	Date              Time   `json:"TglPengumuman"`
	ID                string `json:"Id2"`
	Ticker            string `json:"Kode_Emiten"`
	NoPengumuman      string `json:"NoPengumuman"`
	Title             string `json:"JudulPengumuman"`
	PerihalPengumuman string `json:"PerihalPengumuman"`
}
