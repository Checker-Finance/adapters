package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseServiceConfig_Valid(t *testing.T) {
	m := map[string]string{
		"auth0_endpoint": "https://dev-er8o7vv4aka08m70.us.auth0.com/oauth/token",
		"auth0_audience": "https://dev.xfx.io",
	}
	cfg, err := ParseServiceConfig(m)
	require.NoError(t, err)
	assert.Equal(t, "https://dev-er8o7vv4aka08m70.us.auth0.com/oauth/token", cfg.Auth0Endpoint)
	assert.Equal(t, "https://dev.xfx.io", cfg.Auth0Audience)
}

func TestParseServiceConfig_MissingEndpoint(t *testing.T) {
	m := map[string]string{
		"auth0_audience": "https://dev.xfx.io",
	}
	_, err := ParseServiceConfig(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth0_endpoint")
}

func TestParseServiceConfig_MissingAudience(t *testing.T) {
	m := map[string]string{
		"auth0_endpoint": "https://dev-er8o7vv4aka08m70.us.auth0.com/oauth/token",
	}
	_, err := ParseServiceConfig(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth0_audience")
}
