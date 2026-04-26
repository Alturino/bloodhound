package config

type Telemetry struct {
	Enabled      bool   `mapstructure:"enabled"       json:"enabled"`
	OTLPEndpoint string `mapstructure:"otlp_endpoint" json:"otlp_endpoint"`
	ServiceName  string `mapstructure:"service_name"  json:"service_name"`
}
