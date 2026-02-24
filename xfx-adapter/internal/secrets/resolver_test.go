package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseXFXConfig is the unexported function under test.
// These tests exercise it directly since this package is the test subject.

func TestParseXFXConfig_Valid(t *testing.T) {
	m := map[string]string{
		"client_id":     "my-client-id",
		"client_secret": "my-client-secret",
		"base_url":      "https://dev-api.xfx.io",
	}

	cfg, err := parseXFXConfig(m)
	require.NoError(t, err)
	assert.Equal(t, "my-client-id", cfg.ClientID)
	assert.Equal(t, "my-client-secret", cfg.ClientSecret)
	assert.Equal(t, "https://dev-api.xfx.io", cfg.BaseURL)
}

func TestParseXFXConfig_MissingClientID(t *testing.T) {
	m := map[string]string{
		"client_secret": "my-client-secret",
		"base_url":      "https://dev-api.xfx.io",
	}

	_, err := parseXFXConfig(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_id")
}

func TestParseXFXConfig_MissingClientSecret(t *testing.T) {
	m := map[string]string{
		"client_id": "my-client-id",
		"base_url":  "https://dev-api.xfx.io",
	}

	_, err := parseXFXConfig(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_secret")
}

func TestParseXFXConfig_MissingBaseURL(t *testing.T) {
	m := map[string]string{
		"client_id":     "my-client-id",
		"client_secret": "my-client-secret",
	}

	_, err := parseXFXConfig(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base_url")
}

func TestParseXFXConfig_EmptyMap(t *testing.T) {
	_, err := parseXFXConfig(map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_id")
}

func TestParseXFXConfig_ExtraFieldsIgnored(t *testing.T) {
	m := map[string]string{
		"client_id":     "my-client-id",
		"client_secret": "my-client-secret",
		"base_url":      "https://dev-api.xfx.io",
		"extra_field":   "this-is-ignored",
	}

	cfg, err := parseXFXConfig(m)
	require.NoError(t, err)
	assert.Equal(t, "my-client-id", cfg.ClientID)
	assert.Equal(t, "https://dev-api.xfx.io", cfg.BaseURL)
}
