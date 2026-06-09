package idx

import (
	"context"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

type AttachmentTask struct {
	Ctx        context.Context
	Attachment *model.Attachments `json:"attachment"`
}
