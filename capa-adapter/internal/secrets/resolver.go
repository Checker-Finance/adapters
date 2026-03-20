package secrets

import (
	"context"
	"fmt"

	"github.com/Checker-Finance/adapters/capa-adapter/internal/capa"
	"github.com/Checker-Finance/adapters/capa-adapter/pkg/config"
	intsecrets "github.com/Checker-Finance/adapters/internal/secrets"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// AWSResolver resolves per-client Capa configuration from AWS Secrets Manager.
// It is a thin wrapper over the generic intsecrets.AWSResolver[capa.CapaClientConfig].
//
// Secret naming convention: {env}/{clientID}/capa
// Secret JSON format:       {"api_key": "...", "base_url": "...", "user_id": "...", "webhook_secret": "..."}
type AWSResolver struct {
	inner *intsecrets.AWSResolver[capa.CapaClientConfig]
}

// NewAWSResolver constructs a Capa-specific config resolver.
func NewAWSResolver(
	cfg config.Config,
	provider pkgsecrets.Provider,
	cache *pkgsecrets.Cache[capa.CapaClientConfig],
) *AWSResolver {
	inner := intsecrets.NewAWSResolver(cfg.Env, "capa", provider, cache)
	return &AWSResolver{inner: inner}
}

// Resolve fetches or caches the CapaClientConfig for a given client ID.
func (r *AWSResolver) Resolve(ctx context.Context, clientID string) (*capa.CapaClientConfig, error) {
	cfg, err := r.inner.Resolve(ctx, clientID, parseCapaConfig)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DiscoverClients lists all client IDs that have Capa secrets configured.
func (r *AWSResolver) DiscoverClients(ctx context.Context) ([]string, error) {
	return r.inner.DiscoverClients(ctx)
}

// parseCapaConfig extracts a CapaClientConfig from the raw AWS secret map.
func parseCapaConfig(m map[string]string) (capa.CapaClientConfig, error) {
	cfg := capa.CapaClientConfig{
		APIKey:           m["api_key"],
		BaseURL:          m["base_url"],
		UserID:           m["user_id"],
		WebhookSecret:    m["webhook_secret"],
		WalletAddress:    m["wallet_address"],
		BlockchainSymbol: m["blockchain_symbol"],
		TokenSymbol:      m["token_symbol"],
		ReceiverID:       m["receiver_id"],
	}
	if cfg.APIKey == "" {
		return capa.CapaClientConfig{}, fmt.Errorf("missing required field 'api_key'")
	}
	if cfg.BaseURL == "" {
		return capa.CapaClientConfig{}, fmt.Errorf("missing required field 'base_url'")
	}
	if cfg.UserID == "" {
		return capa.CapaClientConfig{}, fmt.Errorf("missing required field 'user_id'")
	}
	return cfg, nil
}
