package security

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// SecretsManager provides access to AWS Secrets Manager
type SecretsManager struct {
	client *secretsmanager.Client
	region string
}

// NewSecretsManager creates a new SecretsManager
func NewSecretsManager(ctx context.Context, region string) (*SecretsManager, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)

	return &SecretsManager{
		client: client,
		region: region,
	}, nil
}

// GetAuth retrieves and parses authentication credentials from a secret
func (s *SecretsManager) GetAuth(ctx context.Context, secretName string) (*Auth, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: &secretName,
	}

	result, err := s.client.GetSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret value: %w", err)
	}

	if result.SecretString == nil {
		return nil, fmt.Errorf("secret string is nil")
	}

	var auth Auth
	if err := json.Unmarshal([]byte(*result.SecretString), &auth); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret: %w", err)
	}

	return &auth, nil
}

// GetSecret retrieves a raw secret string
func (s *SecretsManager) GetSecret(ctx context.Context, secretName string) (string, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: &secretName,
	}

	result, err := s.client.GetSecretValue(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get secret value: %w", err)
	}

	if result.SecretString == nil {
		return "", fmt.Errorf("secret string is nil")
	}

	return *result.SecretString, nil
}
