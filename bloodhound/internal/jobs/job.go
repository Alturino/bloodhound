package jobs

import (
	"github.com/Alturino/bloodhound/internal/response"
)

type GetAnnouncementArgs struct {
	Page     int
	PageSize int
	JobID    string
	Keyword  string
	Emiten   string
}

type DownloadAttachmentArgs struct {
	JobID        string                `json:"job_id"`
	Announcement response.Announcement `json:"announcement"`
	Attachment   response.Attachment   `json:"attachment"`
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
