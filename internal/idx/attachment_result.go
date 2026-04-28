package idx

type AttachmentResult struct {
	Index           int
	WorkerID        int
	TotalAttachment int
	Filename        string
	AnnouncementID  string
	Err             error
}
