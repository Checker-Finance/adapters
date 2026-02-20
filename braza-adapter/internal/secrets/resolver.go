package secrets

import (
	"context"
	"fmt"
	"strings"

	"github.com/Checker-Finance/adapters/braza-adapter/pkg/config"
	"go.uber.org/zap"

	pkgsecrets "github.com/Checker-Finance/adapters/braza-adapter/pkg/secrets"
)

// AWSResolver resolves credentials for tenants/clients from AWS Secrets Manager,
// caching them locally to reduce API calls.
type AWSResolver struct {
	logger *zap.Logger
	awsSM  *pkgsecrets.AWSSecretsManagerProvider
	cache  *pkgsecrets.Cache
}

// NewAWSResolver constructs a multi-tenant secret resolver using AWS Secrets Manager and local cache.
func NewAWSResolver(logger *zap.Logger, awsSM *pkgsecrets.AWSSecretsManagerProvider, cache *pkgsecrets.Cache) *AWSResolver {
	return &AWSResolver{
		logger: logger,
		awsSM:  awsSM,
		cache:  cache,
	}
}

func (r *AWSResolver) Key(clientID, venue string) string {
	return strings.ToLower(fmt.Sprintf("%s|%s", clientID, venue))
}

func (r *AWSResolver) SecretName(env, clientID, venue string) string {
	return strings.ToLower(fmt.Sprintf("%s/%s/%s", env, clientID, venue))
}

// Resolve fetches or caches credentials for a given tenant/client/venue triple.
func (r *AWSResolver) Resolve(ctx context.Context, cfg config.Config, clientID, venue string) (pkgsecrets.Credentials, error) {
	key := r.Key(clientID, venue)

	// --- check in-memory cache first ---
	if creds, ok := r.cache.Get(key); ok {
		return creds, nil
	}

	// --- fetch from AWS Secrets Manager ---
	secretName := r.SecretName(cfg.Env, clientID, venue)
	secretMap, err := r.awsSM.GetSecret(ctx, secretName)
	if err != nil {
		r.logger.Warn("aws.secret_fetch_failed",
			zap.String("key", secretName),
			zap.Error(err))
		return pkgsecrets.Credentials{}, err
	}

	// Expect AWS SM secret as JSON: {"username": "...", "password": "..."}
	creds := pkgsecrets.Credentials{
		Username: secretMap["username"],
		Password: secretMap["password"],
	}

	// --- cache locally for next time ---
	r.cache.Put(key, creds)

	r.logger.Info("aws.secret_resolved",
		zap.String("client", clientID),
		zap.String("venue", venue),
	)
	return creds, nil
}
