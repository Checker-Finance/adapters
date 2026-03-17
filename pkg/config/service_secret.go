package config

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// FetchServiceSecret fetches the service-level config secret from AWS Secrets Manager.
// Secret path convention: {env}/{service-name} (e.g. "prod/xfx-adapter").
//
// All keys are normalized to lowercase so that secrets stored with UPPERCASE keys
// (matching environment variable conventions) are looked up consistently.
//
// Returns an empty map (not an error) if the secret does not exist, allowing local
// development environments to degrade gracefully to env var defaults.
func FetchServiceSecret(ctx context.Context, region, secretPath string) (map[string]string, error) {
	provider, err := pkgsecrets.NewAWSProvider(region)
	if err != nil {
		return nil, fmt.Errorf("create AWS provider: %w", err)
	}

	m, err := provider.GetSecret(ctx, secretPath)
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	// Normalize all keys to lowercase so that secrets stored as DATABASE_URL
	// and database_url are treated identically.
	normalized := make(map[string]string, len(m))
	for k, v := range m {
		normalized[strings.ToLower(k)] = v
	}
	return normalized, nil
}
