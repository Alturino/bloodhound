package jobs

import (
	"github.com/Alturino/bloodhound/internal/response"
)

type FetchAnnouncementJob struct {
	Page     int
	PageSize int
	JobID    string
	Keyword  string
	Emiten   string
}

type DownloadJob struct {
	JobID      string
	Emiten     string
	Attachment response.Attachment
}

type DownloadRes struct {
	Err          error
	JobID        string
	Emiten       string
	URL          string
	AttachmentID int
	WorkerID     int
}

type FetchAnnouncementRes struct {
	Err error
	response.IdxResponse
}
