package secrets

import (
	"context"
	"fmt"
	"strings"

	"github.com/Checker-Finance/adapters/rio-adapter/internal/rio"
	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
	"go.uber.org/zap"

	pkgsecrets "github.com/Checker-Finance/adapters/rio-adapter/pkg/secrets"
)

// AWSResolver resolves per-client Rio configuration from AWS Secrets Manager,
// caching results locally to reduce API calls.
//
// Secret naming convention: {env}/{clientID}/rio
// Secret JSON format:       {"api_key": "...", "base_url": "https://...", "country": "US"}
type AWSResolver struct {
	logger   *zap.Logger
	cfg      config.Config
	provider pkgsecrets.Provider
	cache    *pkgsecrets.Cache[rio.RioClientConfig]
	venue    string // e.g. "rio"
}

// NewAWSResolver constructs a multi-tenant config resolver using AWS Secrets Manager and local cache.
func NewAWSResolver(
	logger *zap.Logger,
	cfg config.Config,
	provider pkgsecrets.Provider,
	cache *pkgsecrets.Cache[rio.RioClientConfig],
) *AWSResolver {
	return &AWSResolver{
		logger:   logger,
		cfg:      cfg,
		provider: provider,
		cache:    cache,
		venue:    "rio",
	}
}

// cacheKey builds the in-memory cache key for a client.
func (r *AWSResolver) cacheKey(clientID string) string {
	return strings.ToLower(fmt.Sprintf("%s|%s", clientID, r.venue))
}

// secretName builds the AWS Secrets Manager key for a client.
// Pattern: {env}/{clientID}/rio
func (r *AWSResolver) secretName(clientID string) string {
	return strings.ToLower(fmt.Sprintf("%s/%s/%s", r.cfg.Env, clientID, r.venue))
}

// Resolve fetches or caches the RioClientConfig for a given client ID.
func (r *AWSResolver) Resolve(ctx context.Context, clientID string) (*rio.RioClientConfig, error) {
	key := r.cacheKey(clientID)

	// --- check in-memory cache first ---
	if cfg, ok := r.cache.Get(key); ok {
		return &cfg, nil
	}

	// --- fetch from AWS Secrets Manager ---
	secretName := r.secretName(clientID)
	secretMap, err := r.provider.GetSecret(ctx, secretName)
	if err != nil {
		r.logger.Warn("aws.secret_fetch_failed",
			zap.String("key", secretName),
			zap.Error(err))
		return nil, fmt.Errorf("resolve client config for %q: %w", clientID, err)
	}

	cfg := rio.RioClientConfig{
		APIKey:  secretMap["api_key"],
		BaseURL: secretMap["base_url"],
		Country: secretMap["country"],
	}

	// Validate required fields
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("secret %q missing required field 'api_key'", secretName)
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("secret %q missing required field 'base_url'", secretName)
	}
	if cfg.Country == "" {
		return nil, fmt.Errorf("secret %q missing required field 'country'", secretName)
	}

	// --- cache locally for next time ---
	r.cache.Put(key, cfg)

	r.logger.Info("aws.client_config_resolved",
		zap.String("client", clientID),
		zap.String("venue", r.venue),
		zap.String("base_url", cfg.BaseURL),
		zap.String("country", cfg.Country),
	)
	return &cfg, nil
}

// DiscoverClients lists all client IDs that have Rio secrets configured in AWS Secrets Manager.
// It searches for secrets matching the prefix "{env}/" and ending with "/rio",
// then extracts client IDs from the middle segment.
func (r *AWSResolver) DiscoverClients(ctx context.Context) ([]string, error) {
	prefix := strings.ToLower(fmt.Sprintf("%s/", r.cfg.Env))
	suffix := "/" + r.venue

	names, err := r.provider.ListSecrets(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("discover clients: %w", err)
	}

	var clients []string
	for _, name := range names {
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, suffix) {
			continue
		}
		// Extract client ID: "{env}/{clientID}/rio" → clientID
		trimmed := strings.TrimPrefix(lower, prefix)
		trimmed = strings.TrimSuffix(trimmed, suffix)
		if trimmed != "" && !strings.Contains(trimmed, "/") {
			clients = append(clients, trimmed)
		}
	}

	r.logger.Info("aws.clients_discovered",
		zap.Int("count", len(clients)),
		zap.Strings("clients", clients),
	)
	return clients, nil
}
