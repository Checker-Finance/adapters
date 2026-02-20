package store

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/pkg/model"
	"github.com/alicebob/miniredis/v2"
)

func newBenchStore(b *testing.B) (*HybridStore, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	rdb := newRedisClient(mr.Addr())
	return &HybridStore{redis: rdb}, mr
}

func BenchmarkSetBalance(b *testing.B) {
	ctx := context.Background()
	store, mr := newBenchStore(b)
	defer mr.Close()

	bal := model.Balance{
		Venue:       "BRAZA",
		Instrument:  "USDBRL",
		Available:   59992,
		CanBuy:      true,
		CanSell:     true,
		LastUpdated: time.Now(),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bal.Available = float64(i)
		if err := store.SetJSON(ctx, "balance:tenantA:client1:BRAZA:USDBRL", bal, 5*time.Minute); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetBalance(b *testing.B) {
	ctx := context.Background()
	store, mr := newBenchStore(b)
	defer mr.Close()

	bal := model.Balance{
		Venue:       "BRAZA",
		Instrument:  "USDBRL",
		Available:   10000,
		LastUpdated: time.Now(),
	}
	data, _ := json.Marshal(bal)
	mr.Set("balance:tenantA:client1:BRAZA:USDBRL", string(data))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetBalance(ctx, "tenantA", "client1", "BRAZA", "USDBRL")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSetGetJSON(b *testing.B) {
	ctx := context.Background()
	store, mr := newBenchStore(b)
	defer mr.Close()

	payload := map[string]string{
		"api_key":    "abc123",
		"api_secret": "def456",
	}

	b.Run("SetJSON", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := "client:cred:" + strconv.Itoa(i)
			if err := store.SetJSON(ctx, key, payload, 2*time.Minute); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("GetJSON", func(b *testing.B) {
		_ = store.SetJSON(ctx, "client:cred", payload, 2*time.Minute)
		var got map[string]string

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := store.GetJSON(ctx, "client:cred", &got); err != nil {
				b.Fatal(err)
			}
		}
	})
}
