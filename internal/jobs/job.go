package jobs

type DownloadJob struct {
	URL          string
	Filename     string
	Emiten       string
	AttachmentID int
	ReplyID      int
}
