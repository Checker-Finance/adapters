package auth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/pkg/secrets"
)

// CacheAdapter bridges the in-memory secrets.Cache to the Manager’s expected API.
// It can store both credentials and short-lived token bundles as JSON.
type CacheAdapter struct {
	Local *secrets.Cache
}

func NewCacheAdapter(local *secrets.Cache) *CacheAdapter {
	return &CacheAdapter{Local: local}
}

// Get retrieves a cached JSON string value by key.
func (c *CacheAdapter) Get(ctx context.Context, key string) (string, error) {
	if val, ok := c.Local.Get(key); ok {
		b, _ := json.Marshal(val)
		return string(b), nil
	}
	return "", nil
}

// SetWithTTL stores a JSON string value with TTL (used for tokens).
func (c *CacheAdapter) SetWithTTL(ctx context.Context, key, value string, ttl time.Duration) error {
	var tb TokenBundle
	if err := json.Unmarshal([]byte(value), &tb); err != nil {
		return err
	}

	// Wrap token bundle fields in a generic struct for caching
	tokenCreds := secrets.Credentials{
		Username: tb.AccessToken,  // overloaded field for simplicity
		Password: tb.RefreshToken, // overloaded field for simplicity
	}

	c.Local.Put(key, tokenCreds)
	return nil
}
