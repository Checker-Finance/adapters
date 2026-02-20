package secrets

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// AWSSecretsManagerProvider implements Provider using AWS Secrets Manager.
type AWSSecretsManagerProvider struct {
	client *secretsmanager.Client
}

// NewAWSProvider creates a new AWS Secrets Manager provider for the given region.
func NewAWSProvider(region string) (Provider, error) {
	cfg, err := LoadAWSConfig(region)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	return &AWSSecretsManagerProvider{client: client}, nil
}

// GetSecret fetches and decodes a secret value from AWS Secrets Manager.
// Secrets should be stored as JSON maps (e.g. {"api_key": "abc", "base_url": "https://..."}).
func (p *AWSSecretsManagerProvider) GetSecret(ctx context.Context, key string) (map[string]string, error) {
	out, err := p.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch secret [%s]: %w", key, err)
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(*out.SecretString), &result); err != nil {
		return nil, fmt.Errorf("invalid secret format for [%s]: %w", key, err)
	}
	return result, nil
}

// ListSecrets returns the names of all secrets whose name starts with the given prefix.
// It paginates through all results automatically.
func (p *AWSSecretsManagerProvider) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	var names []string
	input := &secretsmanager.ListSecretsInput{
		Filters: []types.Filter{
			{
				Key:    types.FilterNameStringTypeName,
				Values: []string{prefix},
			},
		},
		MaxResults: aws.Int32(100),
	}

	paginator := secretsmanager.NewListSecretsPaginator(p.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets with prefix [%s]: %w", prefix, err)
		}
		for _, entry := range page.SecretList {
			if entry.Name != nil {
				names = append(names, *entry.Name)
			}
		}
	}
	return names, nil
}

func LoadAWSConfig(region string) (aws.Config, error) {
	return config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
}
