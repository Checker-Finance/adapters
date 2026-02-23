package secrets

import (
	"sync"
	"time"
)

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type cacheItem[T any] struct {
	value      T
	expiration time.Time
}

// Cache is a simple thread-safe TTL cache for storing values in-memory.
// It is parameterised on value type so it can hold Credentials, RioClientConfig, etc.
type Cache[T any] struct {
	mu   sync.RWMutex
	data map[string]cacheItem[T]
	ttl  time.Duration
}

// NewCache creates a new TTL-based in-memory cache.
func NewCache[T any](defaultTTL time.Duration) *Cache[T] {
	return &Cache[T]{
		data: make(map[string]cacheItem[T]),
		ttl:  defaultTTL,
	}
}

// Get returns a cached value if present and not expired.
func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	item, ok := c.data[key]
	c.mu.RUnlock()
	if !ok {
		var zero T
		return zero, false
	}
	if time.Now().After(item.expiration) {
		// Expired — remove and miss
		c.mu.Lock()
		delete(c.data, key)
		c.mu.Unlock()
		var zero T
		return zero, false
	}
	return item.value, true
}

// Put inserts or overwrites a cache entry with TTL.
func (c *Cache[T]) Put(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = cacheItem[T]{
		value:      value,
		expiration: time.Now().Add(c.ttl),
	}
}

// Bust deletes a single entry from the cache (e.g., on secret rotation).
func (c *Cache[T]) Bust(key string) {
	c.mu.Lock()
	delete(c.data, key)
	c.mu.Unlock()
}

// StartCleaner periodically removes expired cache entries.
func (c *Cache[T]) StartCleaner(interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-stop:
			return
		}
	}
}

func (c *Cache[T]) cleanupExpired() {
	now := time.Now()
	c.mu.Lock()
	for k, v := range c.data {
		if now.After(v.expiration) {
			delete(c.data, k)
		}
	}
	c.mu.Unlock()
}
