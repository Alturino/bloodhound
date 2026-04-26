package config

type IDX struct {
	PageSize int    `mapstructure:"page_size" json:"page_size"`
	BaseURL  string `mapstructure:"base_url"  json:"base_url"`
	Token    string `mapstructure:"token"     json:"token"`
	MockMode bool   `mapstructure:"mock_mode" json:"mock_mode"`
}
