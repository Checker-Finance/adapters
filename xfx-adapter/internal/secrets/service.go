package secrets

import (
	"context"
	"fmt"

	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
	"github.com/Checker-Finance/adapters/xfx-adapter/internal/xfx"
)

// ParseServiceConfig extracts an XFXServiceConfig from the raw AWS secret map.
func ParseServiceConfig(m map[string]string) (xfx.XFXServiceConfig, error) {
	cfg := xfx.XFXServiceConfig{
		Auth0Endpoint: m["auth0_endpoint"],
		Auth0Audience: m["auth0_audience"],
	}
	if cfg.Auth0Endpoint == "" {
		return xfx.XFXServiceConfig{}, fmt.Errorf("missing required field 'auth0_endpoint'")
	}
	if cfg.Auth0Audience == "" {
		return xfx.XFXServiceConfig{}, fmt.Errorf("missing required field 'auth0_audience'")
	}
	return cfg, nil
}

// FetchServiceConfig fetches the service-level XFX config from AWS Secrets Manager.
// Secret path: {env}/xfx-adapter
func FetchServiceConfig(ctx context.Context, provider pkgsecrets.Provider, env string) (xfx.XFXServiceConfig, error) {
	secretKey := env + "/xfx-adapter"
	m, err := provider.GetSecret(ctx, secretKey)
	if err != nil {
		return xfx.XFXServiceConfig{}, fmt.Errorf("fetch service config %q: %w", secretKey, err)
	}
	return ParseServiceConfig(m)
}
