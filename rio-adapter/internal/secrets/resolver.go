package secrets

import (
	"context"
	"fmt"

	"github.com/Checker-Finance/adapters/rio-adapter/internal/rio"
	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
	intsecrets "github.com/Checker-Finance/adapters/internal/secrets"
	"go.uber.org/zap"
)

// AWSResolver resolves per-client Rio configuration from AWS Secrets Manager.
// It is a thin wrapper over the generic intsecrets.AWSResolver[rio.RioClientConfig].
//
// Secret naming convention: {env}/{clientID}/rio
// Secret JSON format:       {"api_key": "...", "base_url": "https://...", "country": "US"}
type AWSResolver struct {
	inner *intsecrets.AWSResolver[rio.RioClientConfig]
}

// NewAWSResolver constructs a Rio-specific config resolver using AWS Secrets Manager and local cache.
func NewAWSResolver(
	logger *zap.Logger,
	cfg config.Config,
	provider pkgsecrets.Provider,
	cache *pkgsecrets.Cache[rio.RioClientConfig],
) *AWSResolver {
	inner := intsecrets.NewAWSResolver(logger, cfg.Env, "rio", provider, cache)
	return &AWSResolver{inner: inner}
}

// Resolve fetches or caches the RioClientConfig for a given client ID.
func (r *AWSResolver) Resolve(ctx context.Context, clientID string) (*rio.RioClientConfig, error) {
	cfg, err := r.inner.Resolve(ctx, clientID, parseRioConfig)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DiscoverClients lists all client IDs that have Rio secrets configured in AWS Secrets Manager.
func (r *AWSResolver) DiscoverClients(ctx context.Context) ([]string, error) {
	return r.inner.DiscoverClients(ctx)
}

// parseRioConfig extracts a RioClientConfig from the raw AWS secret map.
func parseRioConfig(m map[string]string) (rio.RioClientConfig, error) {
	cfg := rio.RioClientConfig{
		APIKey:  m["api_key"],
		BaseURL: m["base_url"],
		Country: m["country"],
	}
	if cfg.APIKey == "" {
		return rio.RioClientConfig{}, fmt.Errorf("missing required field 'api_key'")
	}
	if cfg.BaseURL == "" {
		return rio.RioClientConfig{}, fmt.Errorf("missing required field 'base_url'")
	}
	if cfg.Country == "" {
		return rio.RioClientConfig{}, fmt.Errorf("missing required field 'country'")
	}
	return cfg, nil
}
