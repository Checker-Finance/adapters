package secrets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// AWSResolver resolves per-client configuration from AWS Secrets Manager,
// caching results locally to reduce API calls. It is generic over the
// resolved config type T so the same core logic can serve all adapters.
//
// Secret naming convention: {env}/{clientID}/{venue}
type AWSResolver[T any] struct {
	env      string
	venue    string
	provider pkgsecrets.Provider
	cache    *pkgsecrets.Cache[T]
}

// NewAWSResolver constructs a generic multi-tenant config resolver.
func NewAWSResolver[T any](
	env string,
	venue string,
	provider pkgsecrets.Provider,
	cache *pkgsecrets.Cache[T],
) *AWSResolver[T] {
	return &AWSResolver[T]{
		env:      env,
		venue:    venue,
		provider: provider,
		cache:    cache,
	}
}

// cacheKey builds the in-memory cache key for a client.
func (r *AWSResolver[T]) cacheKey(clientID string) string {
	return strings.ToLower(fmt.Sprintf("%s|%s", clientID, r.venue))
}

// secretName builds the AWS Secrets Manager key for a client.
// Pattern: {env}/{clientID}/{venue}
func (r *AWSResolver[T]) secretName(clientID string) string {
	return strings.ToLower(fmt.Sprintf("%s/%s/%s", r.env, clientID, r.venue))
}

// Resolve fetches or caches config T for a given client ID.
// parse extracts T from the raw secret map; it should validate required fields.
func (r *AWSResolver[T]) Resolve(ctx context.Context, clientID string, parse func(map[string]string) (T, error)) (T, error) {
	key := r.cacheKey(clientID)

	// --- check in-memory cache first ---
	if cfg, ok := r.cache.Get(key); ok {
		return cfg, nil
	}

	// --- fetch from AWS Secrets Manager ---
	secretName := r.secretName(clientID)
	secretMap, err := r.provider.GetSecret(ctx, secretName)
	if err != nil {
		slog.Warn("aws.secret_fetch_failed",
			"key", secretName,
			"error", err)
		var zero T
		return zero, fmt.Errorf("resolve client config for %q: %w", clientID, err)
	}

	cfg, err := parse(secretMap)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("parse secret %q: %w", secretName, err)
	}

	// --- cache locally for next time ---
	r.cache.Put(key, cfg)

	slog.Info("aws.client_config_resolved",
		"client", clientID,
		"venue", r.venue,
	)
	return cfg, nil
}

// DiscoverClients lists all client IDs that have secrets configured in AWS Secrets Manager.
// It searches for secrets matching the prefix "{env}/" and ending with "/{venue}",
// then extracts client IDs from the middle segment.
func (r *AWSResolver[T]) DiscoverClients(ctx context.Context) ([]string, error) {
	prefix := strings.ToLower(fmt.Sprintf("%s/", r.env))
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
		// Extract client ID: "{env}/{clientID}/{venue}" → clientID
		trimmed := strings.TrimPrefix(lower, prefix)
		trimmed = strings.TrimSuffix(trimmed, suffix)
		if trimmed != "" && !strings.Contains(trimmed, "/") {
			clients = append(clients, trimmed)
		}
	}

	slog.Info("aws.clients_discovered",
		"count", len(clients),
		"clients", clients,
	)
	return clients, nil
}
