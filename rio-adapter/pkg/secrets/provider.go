package secrets

import "context"

// Provider defines a generic secrets manager interface.
// Concrete implementations (AWS, GCP, etc.) can satisfy this.
type Provider interface {
	// GetSecret retrieves a secret by key/path and returns a key-value map.
	GetSecret(ctx context.Context, key string) (map[string]string, error)

	// ListSecrets returns the names of all secrets whose name matches the given prefix.
	ListSecrets(ctx context.Context, prefix string) ([]string, error)
}
