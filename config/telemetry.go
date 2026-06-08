package config

type Telemetry struct {
	Enabled             bool   `mapstructure:"enabled"               json:"enabled"`
	OTLPTracesEndpoint  string `mapstructure:"otlp_traces_endpoint"  json:"otlp_traces_endpoint"`
	OTLPMetricsEndpoint string `mapstructure:"otlp_metrics_endpoint" json:"otlp_metrics_endpoint"`
	PrometheusEndpoint  string `mapstructure:"prometheus_endpoint"   json:"prometheus_endpoint"`
	ServiceName         string `mapstructure:"service_name"          json:"service_name"`
}
