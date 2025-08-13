package jobs

type DownloadRes struct {
	Err          error
	URL          string
	AttachmentID int
	ReplyID      int
	WorkerID     int
}
