package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateDirectOnCallAPI(t *testing.T) {
	cfg := &Config{
		OnCallAPIURL: "https://oncall.example.com",
		OnCallAPIKey: "oncall-key",
	}

	assert.NoError(t, cfg.Validate())
	assert.True(t, cfg.UsesDirectOnCallAPI())
}

func TestValidateDirectOnCallAPIRequiresURLAndKey(t *testing.T) {
	cfg := &Config{OnCallAPIURL: "https://oncall.example.com"}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GRAFANA_ONCALL_API_URL and GRAFANA_ONCALL_API_KEY")
}

func TestValidateGrafanaModeStillRequiresGrafanaURLAndToken(t *testing.T) {
	cfg := &Config{}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GRAFANA_URL is required")
}
