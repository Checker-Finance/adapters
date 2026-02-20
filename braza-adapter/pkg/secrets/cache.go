package secrets

import (
	"sync"
	"time"
)

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type cacheItem struct {
	value      Credentials
	expiration time.Time
}

// Cache is a simple thread-safe TTL cache for storing secrets in-memory.
// Key: tenant_id|client_id|venue (case-insensitive).
type Cache struct {
	mu   sync.RWMutex
	data map[string]cacheItem
	ttl  time.Duration
}

// NewCache creates a new TTL-based in-memory secret cache.
func NewCache(defaultTTL time.Duration) *Cache {
	return &Cache{
		data: make(map[string]cacheItem),
		ttl:  defaultTTL,
	}
}

// Get returns a cached value if present and not expired.
func (c *Cache) Get(key string) (Credentials, bool) {
	c.mu.RLock()
	item, ok := c.data[key]
	c.mu.RUnlock()
	if !ok {
		return Credentials{}, false
	}
	if time.Now().After(item.expiration) {
		// Expired — remove and miss
		c.mu.Lock()
		delete(c.data, key)
		c.mu.Unlock()
		return Credentials{}, false
	}
	return item.value, true
}

// Put inserts or overwrites a cache entry with TTL.
func (c *Cache) Put(key string, value Credentials) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = cacheItem{
		value:      value,
		expiration: time.Now().Add(c.ttl),
	}
}

// Bust deletes a single entry from the cache (e.g., on secret rotation).
func (c *Cache) Bust(key string) {
	c.mu.Lock()
	delete(c.data, key)
	c.mu.Unlock()
}

// StartCleaner periodically removes expired cache entries.
func (c *Cache) StartCleaner(interval time.Duration, stop <-chan struct{}) {
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

func (c *Cache) cleanupExpired() {
	now := time.Now()
	c.mu.Lock()
	for k, v := range c.data {
		if now.After(v.expiration) {
			delete(c.data, k)
		}
	}
	c.mu.Unlock()
}
