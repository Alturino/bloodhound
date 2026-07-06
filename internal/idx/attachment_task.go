package idx

import (
	"context"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

type AttachmentTask struct {
	Ctx        context.Context   `json:"-"`
	Attachment model.Attachments `json:"attachment"`
}
