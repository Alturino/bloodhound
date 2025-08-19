package jobs

import "github.com/Alturino/bloodhound/internal/response"

type DownloadRes struct {
	Err          error
	URL          string
	AttachmentID int
	WorkerID     int
}

type FetchAnnouncementRes struct {
	Err error
	response.Response
}
