package config

type IDX struct {
	PageSize   int         `mapstructure:"page_size"   json:"page_size"`
	BaseURL    string      `mapstructure:"base_url"    json:"base_url"`
	WorkerPool *WorkerPool `mapstructure:"worker_pool" json:"worker_pool"`
	Scheduler  *Scheduler  `mapstructure:"scheduler"   json:"scheduler"`
}

type WorkerPool struct {
	AnnouncementWorkers int `mapstructure:"announcement_workers" json:"announcement_workers"`
	AttachmentWorkers   int `mapstructure:"attachment_workers"   json:"attachment_workers"`
}
