package secrets_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	intsecrets "github.com/Checker-Finance/adapters/internal/secrets"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// ─── mock provider ────────────────────────────────────────────────────────────

type mockProvider struct {
	secrets map[string]map[string]string
	listErr error
	getErr  map[string]error
}

func (m *mockProvider) GetSecret(_ context.Context, key string) (map[string]string, error) {
	if err, ok := m.getErr[key]; ok {
		return nil, err
	}
	if v, ok := m.secrets[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("secret %q not found", key)
}

func (m *mockProvider) ListSecrets(_ context.Context, prefix string) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var names []string
	for k := range m.secrets {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			names = append(names, k)
		}
	}
	return names, nil
}

// ─── test config type ─────────────────────────────────────────────────────────

type testConfig struct {
	APIKey  string
	BaseURL string
}

func parseTestConfig(m map[string]string) (testConfig, error) {
	key := m["api_key"]
	url := m["base_url"]
	if key == "" {
		return testConfig{}, errors.New("missing api_key")
	}
	return testConfig{APIKey: key, BaseURL: url}, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newResolver(provider pkgsecrets.Provider) *intsecrets.AWSResolver[testConfig] {
	cache := pkgsecrets.NewCache[testConfig](time.Minute)
	return intsecrets.NewAWSResolver[testConfig]("prod", "rio", provider, cache)
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestAWSResolver_Resolve_Success(t *testing.T) {
	provider := &mockProvider{
		secrets: map[string]map[string]string{
			"prod/client-a/rio": {"api_key": "key-123", "base_url": "https://api.example.com"},
		},
	}
	r := newResolver(provider)

	cfg, err := r.Resolve(context.Background(), "client-a", parseTestConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "key-123" {
		t.Errorf("got APIKey=%q, want %q", cfg.APIKey, "key-123")
	}
}

func TestAWSResolver_Resolve_CacheHit(t *testing.T) {
	callCount := 0
	provider := &mockProvider{
		secrets: map[string]map[string]string{
			"prod/client-a/rio": {"api_key": "key-123"},
		},
	}
	// Wrap to count calls
	counting := &countingProvider{Provider: provider, count: &callCount}
	r := newResolver(counting)

	ctx := context.Background()
	_, _ = r.Resolve(ctx, "client-a", parseTestConfig) // first call — cache miss
	_, _ = r.Resolve(ctx, "client-a", parseTestConfig) // second call — cache hit

	if callCount != 1 {
		t.Errorf("expected exactly 1 GetSecret call, got %d", callCount)
	}
}

func TestAWSResolver_Resolve_SecretNotFound(t *testing.T) {
	provider := &mockProvider{
		secrets: map[string]map[string]string{},
	}
	r := newResolver(provider)

	_, err := r.Resolve(context.Background(), "unknown-client", parseTestConfig)
	if err == nil {
		t.Fatal("expected error for unknown client, got nil")
	}
}

func TestAWSResolver_Resolve_ProviderError(t *testing.T) {
	provider := &mockProvider{
		secrets: map[string]map[string]string{},
		getErr: map[string]error{
			"prod/client-a/rio": errors.New("AWS timeout"),
		},
	}
	r := newResolver(provider)

	_, err := r.Resolve(context.Background(), "client-a", parseTestConfig)
	if err == nil {
		t.Fatal("expected error when provider fails, got nil")
	}
}

func TestAWSResolver_Resolve_ParseError(t *testing.T) {
	provider := &mockProvider{
		secrets: map[string]map[string]string{
			"prod/client-a/rio": {"base_url": "https://api.example.com"}, // missing api_key
		},
	}
	r := newResolver(provider)

	_, err := r.Resolve(context.Background(), "client-a", parseTestConfig)
	if err == nil {
		t.Fatal("expected parse error for missing api_key, got nil")
	}
}

func TestAWSResolver_DiscoverClients(t *testing.T) {
	provider := &mockProvider{
		secrets: map[string]map[string]string{
			"prod/client-a/rio":   {"api_key": "k1"},
			"prod/client-b/rio":   {"api_key": "k2"},
			"prod/client-b/braza": {"api_key": "k3"}, // different venue — should be excluded
			"dev/client-c/rio":    {"api_key": "k4"}, // different env — should be excluded
		},
	}
	r := newResolver(provider)

	clients, err := r.DiscoverClients(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clients) != 2 {
		t.Errorf("expected 2 clients, got %d: %v", len(clients), clients)
	}
	found := map[string]bool{}
	for _, c := range clients {
		found[c] = true
	}
	if !found["client-a"] || !found["client-b"] {
		t.Errorf("expected client-a and client-b, got %v", clients)
	}
}

func TestAWSResolver_DiscoverClients_ListError(t *testing.T) {
	provider := &mockProvider{
		listErr: errors.New("AWS API error"),
	}
	r := newResolver(provider)

	_, err := r.DiscoverClients(context.Background())
	if err == nil {
		t.Fatal("expected error on list failure, got nil")
	}
}

// ─── counting provider ────────────────────────────────────────────────────────

type countingProvider struct {
	pkgsecrets.Provider
	count *int
}

func (c *countingProvider) GetSecret(ctx context.Context, key string) (map[string]string, error) {
	*c.count++
	return c.Provider.GetSecret(ctx, key)
}
