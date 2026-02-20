package secrets

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// BenchmarkCacheGetPut measures the performance of Get/Put operations
// under concurrent load and varying hit/miss ratios.
func BenchmarkCacheGetPut(b *testing.B) {
	cache := NewCache(10 * time.Second)
	keyPrefix := "tenantA|client|BRAZA"

	// Pre-fill some entries to create partial hit ratios
	for i := 0; i < 1000; i++ {
		key := keyPrefix + strconv.Itoa(i)
		cache.Put(key, Credentials{Username: "key-" + strconv.Itoa(i)})
	}

	b.Run("sequential_hits", func(b *testing.B) {
		key := keyPrefix + "500"
		for i := 0; i < b.N; i++ {
			cache.Get(key)
		}
	})

	b.Run("sequential_puts", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cache.Put(keyPrefix+strconv.Itoa(i%1000), sampleCreds())
		}
	})

	b.Run("concurrent_gets", func(b *testing.B) {
		var wg sync.WaitGroup
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				key := keyPrefix + strconv.Itoa(i%1000)
				cache.Get(key)
			}(n)
		}
		wg.Wait()
	})

	b.Run("concurrent_mixed", func(b *testing.B) {
		var wg sync.WaitGroup
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if i%2 == 0 {
					cache.Get(keyPrefix + strconv.Itoa(i%1000))
				} else {
					cache.Put(keyPrefix+strconv.Itoa(i%1000), sampleCreds())
				}
			}(n)
		}
		wg.Wait()
	})
}

// BenchmarkCacheCleanup measures cleanup throughput.
func BenchmarkCacheCleanup(b *testing.B) {
	cache := NewCache(1 * time.Second)
	for i := 0; i < 10000; i++ {
		cache.Put("key"+strconv.Itoa(i), sampleCreds())
	}

	time.Sleep(1100 * time.Millisecond)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cache.cleanupExpired()
	}
}
