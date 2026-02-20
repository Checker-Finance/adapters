package store

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Checker-Finance/adapters/rio-adapter/pkg/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T) (*HybridStore, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &HybridStore{redis: rdb}, mr
}

func TestSetAndGetJSON(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	val := map[string]string{"api_key": "abc123", "api_secret": "xyz456"}

	if err := store.SetJSON(ctx, "client:cred", val, time.Minute); err != nil {
		t.Fatalf("SetJSON failed: %v", err)
	}

	var got map[string]string
	if err := store.GetJSON(ctx, "client:cred", &got); err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}

	if got["api_key"] != "abc123" {
		t.Errorf("expected api_key=abc123, got %s", got["api_key"])
	}
}

func TestGetBalance_FromRedisCache(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	bal := model.Balance{
		ID:          1,
		ClientID:    "client1",
		Venue:       "RIO",
		Instrument:  "USDBRL",
		Available:   59992,
		CanBuy:      true,
		CanSell:     true,
		LastUpdated: time.Now().UTC(),
	}

	// Set balance directly in Redis
	key := "balance:tenantA:client1:RIO:USDBRL"
	data, _ := json.Marshal(bal)
	_ = mr.Set(key, string(data))

	res, err := store.GetBalance(ctx, "tenantA", "client1", "RIO", "USDBRL")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}
	if res == nil {
		t.Fatal("expected balance, got nil")
	}
	if res.Instrument != "USDBRL" {
		t.Errorf("expected instrument=USDBRL, got %s", res.Instrument)
	}
	if !res.CanBuy || !res.CanSell {
		t.Errorf("expected can_buy/can_sell true")
	}
}

func TestGetBalance_CacheMiss(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	// No data in Redis
	res, err := store.GetBalance(ctx, "tenantA", "client1", "RIO", "USDBRL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != nil {
		t.Errorf("expected nil for cache miss, got %+v", res)
	}
}

func TestSetJSON_Expiration(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	val := map[string]string{"key": "value"}
	if err := store.SetJSON(ctx, "test:key", val, 200*time.Millisecond); err != nil {
		t.Fatalf("SetJSON failed: %v", err)
	}

	// Fast forward miniredis time
	mr.FastForward(300 * time.Millisecond)

	var got map[string]string
	err := store.GetJSON(ctx, "test:key", &got)
	if err == nil {
		t.Fatal("expected error for expired key, got nil")
	}
}

func TestConcurrentJSONWrites(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			val := map[string]int{"value": i}
			_ = store.SetJSON(ctx, "concurrent:key", val, time.Minute)
		}(i)
	}
	wg.Wait()

	var got map[string]int
	if err := store.GetJSON(ctx, "concurrent:key", &got); err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}
	// Just verify we got some value back
	if _, ok := got["value"]; !ok {
		t.Fatal("expected value key in result")
	}
}
