package idx

import (
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

type Attachment struct {
	PDFFilename      string `json:"pdf_filename"`      // PDFFilename
	FullSavePath     string `json:"full_save_path"`    // FullSavePath - URL to download
	OriginalFilename string `json:"original_filename"` // OriginalFilename
}

func (a Attachment) ToAttachments(ann model.Announcements) model.Attachments {
	return model.Attachments{
		IdxAnnouncementID: ann.IdxID,
		IdxURL:            a.FullSavePath,
		OriginalFilename:  a.OriginalFilename,
		Filename:          a.PDFFilename,
		Title:             ann.Title,
		StockCode:         ann.StockCode,
		Date:              ann.Date,
	}
}

func isPDF(att Attachment) bool {
	return strings.EqualFold(filepath.Ext(att.OriginalFilename), ".pdf") ||
		strings.EqualFold(filepath.Ext(att.PDFFilename), ".pdf")
}

func (a Attachment) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("pdf_filename", a.PDFFilename),
		slog.String("full_save_path", a.FullSavePath),
		slog.String("original_filename", a.OriginalFilename),
	)
}
