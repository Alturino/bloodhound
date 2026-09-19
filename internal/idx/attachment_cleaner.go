package idx

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type AttachmentPathCleaner interface {
	Clean(context.Context, AttachmentTask) string
}

func NewAttachmentPathCleaner() AttachmentPathCleaner {
	return &attachmentPathCleaner{
		cleanRegex:      regexp.MustCompile(`[\s_\-+,.;:()]+`),
		whitespaceRegex: regexp.MustCompile(`\s+`),
	}
}

type attachmentPathCleaner struct {
	whitespaceRegex *regexp.Regexp
	cleanRegex      *regexp.Regexp
}

func (a *attachmentPathCleaner) Clean(ctx context.Context, task AttachmentTask) string {
	title := a.cleanRegex.ReplaceAllString(task.Attachment.Title, " ")
	title = a.whitespaceRegex.ReplaceAllString(title, "_")
	title = filepath.Clean(title)
	title = strings.TrimSpace(title)
	title = strings.ToLower(title)
	if len(title) > 64 {
		title = title[:64]
	}
	date := task.Attachment.Date.Format("2006-01-02")
	title = fmt.Sprintf("%s_%s", date, title)

	originalname := a.cleanRegex.ReplaceAllString(task.Attachment.OriginalFilename, " ")
	originalname = a.whitespaceRegex.ReplaceAllString(originalname, "_")
	originalname = strings.ToLower(originalname)
	originalname = strings.TrimSpace(originalname)
	originalname = filepath.Clean(originalname)

	filename := fmt.Sprintf("%s_%s", date, originalname)
	filename = filepath.Clean(filename)

	stockcode := strings.ToLower(task.Attachment.StockCode)
	filePath := filepath.Join(stockcode, title, filename)

	return filePath
}
