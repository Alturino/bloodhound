package idx

import (
	"time"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

type AttachmentResult struct {
	IsDownloaded   bool
	ChecksumSHA256 string
	StoragePath    string
	Filename       string
	Err            error
	UploadedAt     time.Time
	AttachmentTask
}

func (a AttachmentResult) ToAttachments() model.Attachments {
	return model.Attachments{
		IdxAnnouncementID: a.AnnouncementID,
		IdxURL:            a.Attachment.FullSavePath,
		Error: func(err error) string {
			if err == nil {
				return ""
			}
			return err.Error()
		}(a.Err),
		OriginalFilename: a.Attachment.OriginalFilename,
		Filename:         a.Filename,
		Checksum:         a.ChecksumSHA256,
		StoragePath:      a.StoragePath,
		IsDownloaded:     a.IsDownloaded,
		UploadedAt:       time.Now(),
	}
}
