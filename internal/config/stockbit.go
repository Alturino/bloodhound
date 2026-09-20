package config

type Stockbit struct {
	Token   string `mapstructure:"token"    json:"token"`
	BaseURL string `mapstructure:"base_url" json:"base_url"`
}
