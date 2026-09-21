package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/otelconf"
)

func TestLoadOTelConfig(t *testing.T) {
	cfg, err := Load("../../bloodhound.yaml")
	require.NoError(t, err)

	assert.True(t, cfg.Telemetry.Enabled)
	assert.NotEmpty(t, cfg.Telemetry.OTelRaw)

	// Verify it can be parsed by otelconf
	otelCfg, err := otelconf.ParseYAML(cfg.Telemetry.OTelRaw)
	require.NoError(t, err)
	assert.Equal(t, "0.3", otelCfg.FileFormat)
	assert.NotNil(t, otelCfg.Resource)
	assert.NotNil(t, otelCfg.TracerProvider)
	assert.NotNil(t, otelCfg.MeterProvider)
	assert.NotNil(t, otelCfg.Propagator)
}
