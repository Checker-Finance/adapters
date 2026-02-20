package secrets

import (
	"sync"
	"testing"
	"time"
)

// helper: creates a sample Creds map
func sampleCreds() Creds {
	return Creds{
		"api_key":    "abc123",
		"api_secret": "def456",
	}
}

func TestCache_PutAndGet(t *testing.T) {
	cache := NewCache(2 * time.Second)
	key := "tenantA|client1|BRAZA"

	// should miss initially
	if _, ok := cache.Get(key); ok {
		t.Fatal("expected miss on empty cache")
	}

	cache.Put(key, sampleCreds())

	// immediate hit
	if creds, ok := cache.Get(key); !ok {
		t.Fatal("expected cache hit")
	} else if creds["api_key"] != "abc123" {
		t.Errorf("expected api_key=abc123, got %s", creds["api_key"])
	}
}

func TestCache_Expiration(t *testing.T) {
	cache := NewCache(500 * time.Millisecond)
	key := "tenantA|client1|BRAZA"
	cache.Put(key, sampleCreds())

	time.Sleep(600 * time.Millisecond)

	if _, ok := cache.Get(key); ok {
		t.Fatal("expected expired cache entry")
	}
}

func TestCache_Bust(t *testing.T) {
	cache := NewCache(5 * time.Second)
	key := "tenantA|client1|BRAZA"
	cache.Put(key, sampleCreds())

	cache.Bust(key)
	if _, ok := cache.Get(key); ok {
		t.Fatal("expected cache miss after bust")
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	cache := NewCache(2 * time.Second)
	key := "tenantA|client1|BRAZA"

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			cache.Put(key, sampleCreds())
			time.Sleep(time.Millisecond * 5)
		}
	}()

	// Reader
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			cache.Get(key)
			time.Sleep(time.Millisecond * 5)
		}
	}()

	wg.Wait()
}

func TestCache_CleanupExpired(t *testing.T) {
	cache := NewCache(200 * time.Millisecond)
	key1 := "tenantA|client1|BRAZA"
	key2 := "tenantA|client2|BRAZA"
	cache.Put(key1, sampleCreds())
	cache.Put(key2, sampleCreds())

	time.Sleep(300 * time.Millisecond)
	cache.cleanupExpired()

	if _, ok := cache.Get(key1); ok {
		t.Fatal("expected key1 expired and cleaned up")
	}
	if _, ok := cache.Get(key2); ok {
		t.Fatal("expected key2 expired and cleaned up")
	}
}
