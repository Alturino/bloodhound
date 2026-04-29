package config

type IDX struct {
	MockMode   bool             `mapstructure:"mock_mode"   json:"mock_mode"`
	PageSize   int              `mapstructure:"page_size"   json:"page_size"`
	BaseURL    string           `mapstructure:"base_url"    json:"base_url"`
	Token      string           `mapstructure:"token"       json:"token"`
	WorkerPool WorkerPoolConfig `mapstructure:"worker_pool" json:"worker_pool"`
}

type WorkerPoolConfig struct {
	PageSize            int `mapstructure:"page_size"            json:"page_size"`
	AnnouncementWorkers int `mapstructure:"announcement_workers" json:"announcement_workers"`
	AttachmentWorkers   int `mapstructure:"attachment_workers"   json:"attachment_workers"`
}
