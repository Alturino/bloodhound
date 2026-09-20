package config

type Storage struct {
	MinIO *MinIO `mapstructure:"minio" json:"min_io"`
	Local *Local `mapstructure:"local" json:"local"`
}

type Local struct {
	Enabled       bool   `mapstructure:"enabled"        json:"enabled"`
	BloodhoundDir string `mapstructure:"bloodhound_dir" json:"bloodhound_dir"`
}
