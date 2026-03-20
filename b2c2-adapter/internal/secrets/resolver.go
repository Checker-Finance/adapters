package secrets

import (
	"context"
	"fmt"

	"github.com/Checker-Finance/adapters/b2c2-adapter/internal/b2c2"
	"github.com/Checker-Finance/adapters/b2c2-adapter/pkg/config"
	intsecrets "github.com/Checker-Finance/adapters/internal/secrets"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// AWSResolver resolves per-client B2C2 configuration from AWS Secrets Manager.
// It is a thin wrapper over the generic intsecrets.AWSResolver[b2c2.B2C2ClientConfig].
//
// Secret naming convention: {env}/{clientID}/b2c2
// Secret JSON format:       {"api_token":"...","base_url":"https://api.b2c2.net"}
type AWSResolver struct {
	inner *intsecrets.AWSResolver[b2c2.B2C2ClientConfig]
	cfg   *config.Config
}

// NewAWSResolver constructs a B2C2-specific config resolver.
func NewAWSResolver(
	cfg *config.Config,
	provider pkgsecrets.Provider,
	cache *pkgsecrets.Cache[b2c2.B2C2ClientConfig],
) *AWSResolver {
	inner := intsecrets.NewAWSResolver(cfg.Env, "b2c2", provider, cache)
	return &AWSResolver{inner: inner, cfg: cfg}
}

// Resolve fetches or caches the B2C2ClientConfig for a given client ID.
func (r *AWSResolver) Resolve(ctx context.Context, clientID string) (*b2c2.B2C2ClientConfig, error) {
	cfg, err := r.inner.Resolve(ctx, clientID, r.parse)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DiscoverClients lists all client IDs that have B2C2 secrets configured.
func (r *AWSResolver) DiscoverClients(ctx context.Context) ([]string, error) {
	return r.inner.DiscoverClients(ctx)
}

// parse extracts a B2C2ClientConfig from the raw AWS secret map.
func (r *AWSResolver) parse(m map[string]string) (b2c2.B2C2ClientConfig, error) {
	cfg := b2c2.B2C2ClientConfig{
		APIToken: m["api_token"],
		BaseURL:  m["base_url"],
	}
	if cfg.APIToken == "" {
		return b2c2.B2C2ClientConfig{}, fmt.Errorf("missing required field 'api_token'")
	}
	// Fall back to the default base URL from config if not specified in the secret.
	if cfg.BaseURL == "" {
		cfg.BaseURL = r.cfg.DefaultBaseURL
	}
	return cfg, nil
}
