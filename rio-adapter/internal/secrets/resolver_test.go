package secrets

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/rio-adapter/internal/rio"
	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// --- Mock Provider ---

type mockProvider struct {
	secrets     map[string]map[string]string
	secretNames []string // for ListSecrets
	err         error
	calls       int
}

func (m *mockProvider) GetSecret(_ context.Context, key string) (map[string]string, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	if v, ok := m.secrets[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("secret not found: %s", key)
}

func (m *mockProvider) ListSecrets(_ context.Context, prefix string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.secretNames, nil
}

// --- Tests ---

func TestAWSResolver_Resolve_CacheHit(t *testing.T) {
	cache := pkgsecrets.NewCache[rio.RioClientConfig](5 * time.Minute)
	cache.Put("client-001|rio", rio.RioClientConfig{
		APIKey:  "cached-key",
		BaseURL: "https://cached.example.com",
		Country: "US",
	})

	mock := &mockProvider{}
	cfg := config.Config{Env: "dev"}
	r := NewAWSResolver(zap.NewNop(), cfg, mock, cache)

	clientCfg, err := r.Resolve(context.Background(), "client-001")

	require.NoError(t, err)
	assert.Equal(t, "cached-key", clientCfg.APIKey)
	assert.Equal(t, "https://cached.example.com", clientCfg.BaseURL)
	assert.Equal(t, "US", clientCfg.Country)
	assert.Equal(t, 0, mock.calls, "should not call provider on cache hit")
}

func TestAWSResolver_Resolve_CacheMiss_FetchFromProvider(t *testing.T) {
	cache := pkgsecrets.NewCache[rio.RioClientConfig](5 * time.Minute)

	mock := &mockProvider{
		secrets: map[string]map[string]string{
			"dev/client-001/rio": {
				"api_key":  "aws-key-123",
				"base_url": "https://rio.example.com",
				"country":  "MX",
			},
		},
	}

	cfg := config.Config{Env: "dev"}
	r := NewAWSResolver(zap.NewNop(), cfg, mock, cache)

	clientCfg, err := r.Resolve(context.Background(), "client-001")

	require.NoError(t, err)
	assert.Equal(t, "aws-key-123", clientCfg.APIKey)
	assert.Equal(t, "https://rio.example.com", clientCfg.BaseURL)
	assert.Equal(t, "MX", clientCfg.Country)
	assert.Equal(t, 1, mock.calls)

	// Second call should hit cache — no additional provider call
	clientCfg2, err := r.Resolve(context.Background(), "client-001")
	require.NoError(t, err)
	assert.Equal(t, "aws-key-123", clientCfg2.APIKey)
	assert.Equal(t, 1, mock.calls, "should not call provider again on cache hit")
}

func TestAWSResolver_Resolve_ProviderError(t *testing.T) {
	cache := pkgsecrets.NewCache[rio.RioClientConfig](5 * time.Minute)

	mock := &mockProvider{
		err: fmt.Errorf("aws: access denied"),
	}

	cfg := config.Config{Env: "dev"}
	r := NewAWSResolver(zap.NewNop(), cfg, mock, cache)

	clientCfg, err := r.Resolve(context.Background(), "client-001")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
	assert.Nil(t, clientCfg)
}

func TestAWSResolver_Resolve_SecretNotFound(t *testing.T) {
	cache := pkgsecrets.NewCache[rio.RioClientConfig](5 * time.Minute)

	mock := &mockProvider{
		secrets: map[string]map[string]string{},
	}

	cfg := config.Config{Env: "dev"}
	r := NewAWSResolver(zap.NewNop(), cfg, mock, cache)

	_, err := r.Resolve(context.Background(), "unknown-client")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret not found")
}

func TestAWSResolver_Resolve_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		secret  map[string]string
		errText string
	}{
		{
			name:    "missing api_key",
			secret:  map[string]string{"base_url": "https://x.com", "country": "US"},
			errText: "api_key",
		},
		{
			name:    "missing base_url",
			secret:  map[string]string{"api_key": "key", "country": "US"},
			errText: "base_url",
		},
		{
			name:    "missing country",
			secret:  map[string]string{"api_key": "key", "base_url": "https://x.com"},
			errText: "country",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockProvider{
				secrets: map[string]map[string]string{
					"dev/client-001/rio": tt.secret,
				},
			}
			cfg := config.Config{Env: "dev"}
			r := NewAWSResolver(zap.NewNop(), cfg, mock, pkgsecrets.NewCache[rio.RioClientConfig](5*time.Minute))

			_, err := r.Resolve(context.Background(), "client-001")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errText)
		})
	}
}

func TestAWSResolver_Resolve_CacheExpiration(t *testing.T) {
	cache := pkgsecrets.NewCache[rio.RioClientConfig](10 * time.Millisecond) // very short TTL

	mock := &mockProvider{
		secrets: map[string]map[string]string{
			"dev/client-001/rio": {
				"api_key":  "key1",
				"base_url": "https://rio.example.com",
				"country":  "US",
			},
		},
	}

	cfg := config.Config{Env: "dev"}
	r := NewAWSResolver(zap.NewNop(), cfg, mock, cache)

	// First call — cache miss, fetch from provider
	_, err := r.Resolve(context.Background(), "client-001")
	require.NoError(t, err)
	assert.Equal(t, 1, mock.calls)

	// Wait for cache to expire
	time.Sleep(20 * time.Millisecond)

	// Second call — cache expired, fetch again
	_, err = r.Resolve(context.Background(), "client-001")
	require.NoError(t, err)
	assert.Equal(t, 2, mock.calls, "should call provider again after cache expiry")
}

func TestAWSResolver_DiscoverClients(t *testing.T) {
	mock := &mockProvider{
		secretNames: []string{
			"dev/client-001/rio",
			"dev/client-002/rio",
			"dev/client-003/braza", // different venue — should be excluded
			"dev/other-thing",      // not a client secret — should be excluded
		},
	}

	cfg := config.Config{Env: "dev"}
	r := NewAWSResolver(zap.NewNop(), cfg, mock, pkgsecrets.NewCache[rio.RioClientConfig](5*time.Minute))

	clients, err := r.DiscoverClients(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"client-001", "client-002"}, clients)
}

func TestAWSResolver_DiscoverClients_Empty(t *testing.T) {
	mock := &mockProvider{
		secretNames: []string{},
	}

	cfg := config.Config{Env: "dev"}
	r := NewAWSResolver(zap.NewNop(), cfg, mock, pkgsecrets.NewCache[rio.RioClientConfig](5*time.Minute))

	clients, err := r.DiscoverClients(context.Background())
	require.NoError(t, err)
	assert.Empty(t, clients)
}

func TestAWSResolver_DiscoverClients_ProviderError(t *testing.T) {
	mock := &mockProvider{
		err: fmt.Errorf("aws: list failed"),
	}

	cfg := config.Config{Env: "dev"}
	r := NewAWSResolver(zap.NewNop(), cfg, mock, pkgsecrets.NewCache[rio.RioClientConfig](5*time.Minute))

	_, err := r.DiscoverClients(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list failed")
}
