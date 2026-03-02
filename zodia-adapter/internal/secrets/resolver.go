package secrets

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/zodia-adapter/internal/zodia"
	"github.com/Checker-Finance/adapters/zodia-adapter/pkg/config"
	intsecrets "github.com/Checker-Finance/adapters/internal/secrets"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// AWSResolver resolves per-client Zodia configuration from AWS Secrets Manager.
// It is a thin wrapper over the generic intsecrets.AWSResolver[zodia.ZodiaClientConfig].
//
// Secret naming convention: {env}/zodia/{clientID}
// Secret JSON format:       {"api_key": "...", "api_secret": "...", "base_url": "https://trade-uk.zodiamarkets.com"}
type AWSResolver struct {
	inner *intsecrets.AWSResolver[zodia.ZodiaClientConfig]
}

// NewAWSResolver constructs a Zodia-specific config resolver.
func NewAWSResolver(
	logger *zap.Logger,
	cfg config.Config,
	provider pkgsecrets.Provider,
	cache *pkgsecrets.Cache[zodia.ZodiaClientConfig],
) *AWSResolver {
	inner := intsecrets.NewAWSResolver(logger, cfg.Env, "zodia", provider, cache)
	return &AWSResolver{inner: inner}
}

// Resolve fetches or caches the ZodiaClientConfig for a given client ID.
func (r *AWSResolver) Resolve(ctx context.Context, clientID string) (*zodia.ZodiaClientConfig, error) {
	cfg, err := r.inner.Resolve(ctx, clientID, parseZodiaConfig)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DiscoverClients lists all client IDs that have Zodia secrets configured.
func (r *AWSResolver) DiscoverClients(ctx context.Context) ([]string, error) {
	return r.inner.DiscoverClients(ctx)
}

// parseZodiaConfig extracts a ZodiaClientConfig from the raw AWS secret map.
func parseZodiaConfig(m map[string]string) (zodia.ZodiaClientConfig, error) {
	cfg := zodia.ZodiaClientConfig{
		APIKey:    m["api_key"],
		APISecret: m["api_secret"],
		BaseURL:   m["base_url"],
	}
	if cfg.APIKey == "" {
		return zodia.ZodiaClientConfig{}, fmt.Errorf("missing required field 'api_key'")
	}
	if cfg.APISecret == "" {
		return zodia.ZodiaClientConfig{}, fmt.Errorf("missing required field 'api_secret'")
	}
	if cfg.BaseURL == "" {
		return zodia.ZodiaClientConfig{}, fmt.Errorf("missing required field 'base_url'")
	}
	return cfg, nil
}
