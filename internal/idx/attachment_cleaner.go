package idx

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type AttachmentPathCleaner interface {
	Clean(context.Context, AttachmentTask) string
}

func NewAttachmentPathCleaner() AttachmentPathCleaner {
	return &attachmentPathCleaner{
		replacer: strings.NewReplacer(
			",", " ",
			"//", " ",
			"..", " ",
			"/", " ",
			"\t", " ",
			";", " ",
			":", " ",
			"(", " ",
			")", " ",
			" ", "_",
		),
	}
}

type attachmentPathCleaner struct {
	replacer *strings.Replacer
}

func (a *attachmentPathCleaner) Clean(ctx context.Context, task AttachmentTask) string {
	originalname := a.replacer.Replace(task.Attachment.OriginalFilename)
	originalname = strings.ToLower(originalname)
	originalname = strings.TrimSpace(originalname)
	originalname = filepath.Clean(originalname)

	date := task.Attachment.Date.Format("2006-01-02")

	stockcode := strings.ToLower(task.Attachment.StockCode)
	title := a.replacer.Replace(task.Attachment.Title)
	title = filepath.Clean(title)
	title = strings.TrimSpace(title)
	title = strings.ToLower(title)
	if len(title) > 64 {
		title = title[:64]
	}
	title = fmt.Sprintf("%s_%s", date, title)

	filename := fmt.Sprintf("%s_%s", date, originalname)
	filename = filepath.Clean(filename)

	filePath := filepath.Join(stockcode, title, filename)

	return filePath
}
