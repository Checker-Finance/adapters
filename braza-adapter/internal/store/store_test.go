package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/pkg/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// helper to get a redis client for miniredis
func newRedisClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr})
}

func newTestStore(t *testing.T) (*HybridStore, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	rdb := newRedisClient(mr.Addr())
	return &HybridStore{redis: rdb}, mr
}

func TestSetAndGetBalance(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	bal := model.Balance{
		Venue:       "BRAZA",
		Instrument:  "USDBRL",
		Available:   59992,
		CanBuy:      true,
		CanSell:     true,
		LastUpdated: time.Now().UTC(),
	}

	if err := store.SetJSON(ctx, "balance:tenantA:client1:BRAZA:USDBRL", bal, 5*time.Minute); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	res, err := store.GetBalance(ctx, "tenantA", "client1", "BRAZA", "USDBRL")
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

func TestBalanceExpiration(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	bal := model.Balance{
		Venue:       "BRAZA",
		Instrument:  "USDBRL",
		Available:   1000,
		LastUpdated: time.Now(),
	}

	if err := store.SetJSON(ctx, "balance:tenantA:client1:BRAZA:USDBRL", bal, 200*time.Millisecond); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	mr.FastForward(300 * time.Millisecond)

	res, err := store.GetBalance(ctx, "tenantA", "client1", "BRAZA", "USDBRL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != nil {
		t.Fatal("expected nil for expired key, got result")
	}
}

func TestConcurrentBalanceWrites(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	const key = "balance:tenantA:client1:BRAZA:USDBRL"
	bal := model.Balance{
		Venue:       "BRAZA",
		Instrument:  "USDBRL",
		Available:   1000,
		LastUpdated: time.Now(),
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b := bal
			b.Available = float64(i)
			_ = store.SetJSON(ctx, key, b, 5*time.Minute)
		}(i)
	}
	wg.Wait()

	res, err := store.GetBalance(ctx, "tenantA", "client1", "BRAZA", "USDBRL")
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}
	if res == nil {
		t.Fatal("expected result after concurrent writes")
	}
}
