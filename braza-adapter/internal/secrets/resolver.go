package secrets

import (
	"context"
	"fmt"

	intsecrets "github.com/Checker-Finance/adapters/internal/secrets"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
	"go.uber.org/zap"
)

// Provider is the interface expected by components needing per-client credential resolution.
type Provider interface {
	Resolve(ctx context.Context, clientID string) (pkgsecrets.Credentials, error)
}

// AWSResolver wraps the generic AWSResolver to provide Braza-specific credential resolution.
type AWSResolver struct {
	inner *intsecrets.AWSResolver[pkgsecrets.Credentials]
}

// NewAWSResolver constructs an AWSResolver for the Braza adapter.
func NewAWSResolver(
	logger *zap.Logger,
	env string,
	provider pkgsecrets.Provider,
	cache *pkgsecrets.Cache[pkgsecrets.Credentials],
) *AWSResolver {
	inner := intsecrets.NewAWSResolver(logger, env, "braza", provider, cache)
	return &AWSResolver{inner: inner}
}

// Resolve fetches or caches Braza credentials for a given client ID.
func (r *AWSResolver) Resolve(ctx context.Context, clientID string) (pkgsecrets.Credentials, error) {
	return r.inner.Resolve(ctx, clientID, parseCredentials)
}

// DiscoverClients lists all client IDs configured in AWS Secrets Manager for Braza.
func (r *AWSResolver) DiscoverClients(ctx context.Context) ([]string, error) {
	return r.inner.DiscoverClients(ctx)
}

// parseCredentials extracts a Credentials value from the raw AWS secret map.
func parseCredentials(m map[string]string) (pkgsecrets.Credentials, error) {
	creds := pkgsecrets.Credentials{
		Username: m["username"],
		Password: m["password"],
	}
	if creds.Username == "" {
		return pkgsecrets.Credentials{}, fmt.Errorf("missing 'username' in secret")
	}
	if creds.Password == "" {
		return pkgsecrets.Credentials{}, fmt.Errorf("missing 'password' in secret")
	}
	return creds, nil
}
