package jobs

import (
	"time"

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
	JobID            string              `json:"job_id"`
	Emiten           string              `json:"emiten"`
	AnnouncementDate time.Time           `json:"announcement_date"`
	Attachment       response.Attachment `json:"attachment"`
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
