package jobs

import (
	"os"
)

type DownloadJob struct {
	URL          string
	File         *os.File
	AttachmentID int
	ReplyID      int
}
