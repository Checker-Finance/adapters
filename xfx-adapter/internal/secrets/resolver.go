package secrets

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/xfx-adapter/internal/xfx"
	"github.com/Checker-Finance/adapters/xfx-adapter/pkg/config"
	intsecrets "github.com/Checker-Finance/adapters/internal/secrets"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// AWSResolver resolves per-client XFX configuration from AWS Secrets Manager.
// It is a thin wrapper over the generic intsecrets.AWSResolver[xfx.XFXClientConfig].
//
// Secret naming convention: {env}/xfx/{clientID}
// Secret JSON format:       {"client_id": "...", "client_secret": "...", "base_url": "https://api.xfx.io"}
type AWSResolver struct {
	inner *intsecrets.AWSResolver[xfx.XFXClientConfig]
}

// NewAWSResolver constructs an XFX-specific config resolver.
func NewAWSResolver(
	logger *zap.Logger,
	cfg config.Config,
	provider pkgsecrets.Provider,
	cache *pkgsecrets.Cache[xfx.XFXClientConfig],
) *AWSResolver {
	inner := intsecrets.NewAWSResolver(logger, cfg.Env, "xfx", provider, cache)
	return &AWSResolver{inner: inner}
}

// Resolve fetches or caches the XFXClientConfig for a given client ID.
func (r *AWSResolver) Resolve(ctx context.Context, clientID string) (*xfx.XFXClientConfig, error) {
	cfg, err := r.inner.Resolve(ctx, clientID, parseXFXConfig)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DiscoverClients lists all client IDs that have XFX secrets configured.
func (r *AWSResolver) DiscoverClients(ctx context.Context) ([]string, error) {
	return r.inner.DiscoverClients(ctx)
}

// parseXFXConfig extracts an XFXClientConfig from the raw AWS secret map.
func parseXFXConfig(m map[string]string) (xfx.XFXClientConfig, error) {
	cfg := xfx.XFXClientConfig{
		ClientID:     m["client_id"],
		ClientSecret: m["client_secret"],
		BaseURL:      m["base_url"],
	}
	if cfg.ClientID == "" {
		return xfx.XFXClientConfig{}, fmt.Errorf("missing required field 'client_id'")
	}
	if cfg.ClientSecret == "" {
		return xfx.XFXClientConfig{}, fmt.Errorf("missing required field 'client_secret'")
	}
	if cfg.BaseURL == "" {
		return xfx.XFXClientConfig{}, fmt.Errorf("missing required field 'base_url'")
	}
	return cfg, nil
}
