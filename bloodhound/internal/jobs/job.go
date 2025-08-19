package jobs

import "github.com/Alturino/bloodhound/internal/response"

type FetchAnnouncementJob struct {
	Page     int
	PageSize int
	Keyword  string
	Emiten   string
}

type DownloadJob struct {
	Emiten     string
	Attacments []response.Attachment
}
