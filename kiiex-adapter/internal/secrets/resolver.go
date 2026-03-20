package secrets

import (
	"context"
	"fmt"
	"strconv"

	intsecrets "github.com/Checker-Finance/adapters/internal/secrets"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/security"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// AWSResolver resolves per-client Kiiex configuration from AWS Secrets Manager.
// It is a thin wrapper over the generic intsecrets.AWSResolver[security.Auth].
//
// Secret naming convention: {env}/{clientID}/kiiex
// Secret JSON format: {"api_key":"...","user_id":"1","nonce":"...","oms_id":"1","account_id":"1","username":"...","secret":"..."}
type AWSResolver struct {
	inner *intsecrets.AWSResolver[security.Auth]
}

// NewAWSResolver constructs a Kiiex-specific config resolver using AWS Secrets Manager and local cache.
func NewAWSResolver(
	env string,
	provider pkgsecrets.Provider,
	cache *pkgsecrets.Cache[security.Auth],
) *AWSResolver {
	inner := intsecrets.NewAWSResolver(env, "kiiex", provider, cache)
	return &AWSResolver{inner: inner}
}

// Resolve fetches or caches the Auth for a given client ID.
func (r *AWSResolver) Resolve(ctx context.Context, clientID string) (*security.Auth, error) {
	cfg, err := r.inner.Resolve(ctx, clientID, parseKiiexConfig)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DiscoverClients lists all client IDs that have Kiiex secrets configured in AWS Secrets Manager.
func (r *AWSResolver) DiscoverClients(ctx context.Context) ([]string, error) {
	return r.inner.DiscoverClients(ctx)
}

// parseKiiexConfig extracts a security.Auth from the raw AWS secret map and generates the HMAC signature.
func parseKiiexConfig(m map[string]string) (security.Auth, error) {
	auth := security.Auth{
		APIKey:   m["api_key"],
		Nonce:    m["nonce"],
		Username: m["username"],
		Secret:   m["secret"],
	}
	if auth.APIKey == "" {
		return security.Auth{}, fmt.Errorf("missing required field 'api_key'")
	}
	if auth.Nonce == "" {
		return security.Auth{}, fmt.Errorf("missing required field 'nonce'")
	}
	if auth.Secret == "" {
		return security.Auth{}, fmt.Errorf("missing required field 'secret'")
	}

	var err error
	if auth.UserID, err = parseIntField(m, "user_id"); err != nil {
		return security.Auth{}, err
	}
	if auth.OmsID, err = parseIntField(m, "oms_id"); err != nil {
		return security.Auth{}, err
	}
	if auth.AccountID, err = parseIntField(m, "account_id"); err != nil {
		return security.Auth{}, err
	}

	if err := auth.GenerateSignature(auth.Secret); err != nil {
		return security.Auth{}, fmt.Errorf("generate signature: %w", err)
	}

	return auth, nil
}

func parseIntField(m map[string]string, key string) (int, error) {
	s, ok := m[key]
	if !ok || s == "" {
		return 0, fmt.Errorf("missing required field %q", key)
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for field %q: %w", key, err)
	}
	return v, nil
}
