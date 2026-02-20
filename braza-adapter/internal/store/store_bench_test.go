package store

import (
	"context"
	"encoding/json"
	"github.com/Checker-Finance/adapters/braza-adapter/pkg/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"strconv"
	"testing"
	"time"
)

func newBenchStore(b *testing.B) (*hybridStore, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	rdb := newRedisClient(mr.Addr())
	return &hybridStore{redis: rdb}, mr
}

func BenchmarkSetBalance(b *testing.B) {
	ctx := context.Background()
	store, mr := newBenchStore(b)
	defer mr.Close()

	bal := model.Balance{
		ID:                  uuid.New(),
		Venue:               "BRAZA",
		Instrument:          "USDBRL",
		AvailableTotalValue: 59992,
		CanBuy:              true,
		CanSell:             true,
		LastUpdated:         time.Now(),
	}

	clientKey := "tenantA|client1"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bal.AvailableTotalValue = float64(i)
		if err := store.SetBalance(ctx, clientKey, bal); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetBalance(b *testing.B) {
	ctx := context.Background()
	store, mr := newBenchStore(b)
	defer mr.Close()

	key := "tenantA|client1"
	bal := model.Balance{
		ID:                  uuid.New(),
		Venue:               "BRAZA",
		Instrument:          "USDBRL",
		AvailableTotalValue: 10000,
		LastUpdated:         time.Now(),
	}
	data, _ := json.Marshal(bal)
	mr.Set("balance:"+key+":USDBRL", string(data))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetBalance(ctx, key, "USDBRL")
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
