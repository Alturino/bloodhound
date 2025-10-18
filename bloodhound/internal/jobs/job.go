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

type DownloadFileArgs struct {
	JobID         string
	Emiten        string
	TglPengumuman time.Time
	Attachment    response.Attachment
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
