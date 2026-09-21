package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadOTelConfig(t *testing.T) {
	cfg, err := Load("../../bloodhound.yaml")
	require.NoError(t, err)

	assert.True(t, cfg.App.Enabled)
	assert.NotNil(t, cfg.Telemetry)
	assert.Equal(t, "0.3", cfg.Telemetry.FileFormat)
	assert.NotNil(t, cfg.Telemetry.Resource)
	assert.NotNil(t, cfg.Telemetry.TracerProvider)
	assert.NotNil(t, cfg.Telemetry.MeterProvider)
	assert.NotNil(t, cfg.Telemetry.Propagator)
}
