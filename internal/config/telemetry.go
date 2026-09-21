package config

type Telemetry struct {
	Enabled bool   `mapstructure:"enabled" json:"enabled"`
	OTelRaw []byte `mapstructure:"-"        json:"-"`
}
