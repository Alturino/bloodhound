package config

type MinIO struct {
	UseSSL    bool   `mapstructure:"use_ssl"    json:"use_ssl"`
	Endpoint  string `mapstructure:"endpoint"   json:"endpoint"`
	AccessKey string `mapstructure:"access_key" json:"access_key"`
	SecretKey string `mapstructure:"secret_key" json:"secret_key"`
	Bucket    string `mapstructure:"bucket"     json:"bucket"`
}
